package llm_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/nilsherzig/LLocalSearch/utils"
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/tools"
)

type WebSearch struct {
	CallbacksHandler callbacks.Handler
	SessionString    string
	Settings         utils.ClientSettings
}

const MaxLinksPerSession = 100 // Prevent unlimited memory growth

var (
	simpleUsedLinks   = make(map[string][]string)
	simpleUsedLinksMu sync.RWMutex
)

var _ tools.Tool = WebSearch{}

func (c WebSearch) Description() string {
	return `Use this tool to search for websites that may answer your search query. A summary of the websites will be returned to you directly. This might be enough to answer simple questions.`
}

func (c WebSearch) Name() string {
	return "websearch"
}

func (ws WebSearch) Call(ctx context.Context, input string) (string, error) {
	if ws.CallbacksHandler != nil {
		ws.CallbacksHandler.HandleToolStart(ctx, input)
	}

	input = strings.TrimPrefix(input, "\"")
	input = strings.TrimSuffix(input, "\"")
	inputQuery := url.QueryEscape(input)
	searXNGDomain := os.Getenv("SEARXNG_DOMAIN")
	url := fmt.Sprintf("%s/?q=%s&format=json", searXNGDomain, inputQuery)
	resp, err := http.Get(url)

	if err != nil {
		slog.Warn("Error making the request", "error", err)
		return "", err
	}
	defer resp.Body.Close()

	var apiResponse utils.SeaXngResult
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		slog.Warn("Error decoding the response", "error", err)
		return "", err
	}

	wg := sync.WaitGroup{}
	counter := 0
	summaryResults := []string{}
	
	simpleUsedLinksMu.RLock()
	sessionLinks := simpleUsedLinks[ws.SessionString]
	simpleUsedLinksMu.RUnlock()
	
	for i := range apiResponse.Results {
		skip := false
		for _, usedLink := range sessionLinks {
			if usedLink == apiResponse.Results[i].URL {
				slog.Warn("Skipping already used link during SimpleWebSearch", "link", apiResponse.Results[i].URL)
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		if counter >= ws.Settings.AmountOfWebsites {
			break
		}

		if strings.HasSuffix(apiResponse.Results[i].URL, ".pdf") {
			continue
		}

		summaryResults = append(summaryResults, apiResponse.Results[i].Content)

		counter += 1
		wg.Add(1)
		go func(i int) {
			defer func() {
				wg.Done()
				if r := recover(); r != nil {
					slog.Error("Recovered from panic", "error", r)
				}
			}()

			ch, ok := ws.CallbacksHandler.(utils.CustomHandler)
			if ok {
				newSource := utils.Source{
					Name:    "SimpleWebSearch",
					Link:    apiResponse.Results[i].URL,
					Summary: apiResponse.Results[i].Content,
					Title:   apiResponse.Results[i].Title,
					Engine:  apiResponse.Results[i].Engine,
				}

				ch.HandleSourceAdded(ctx, newSource)
			}
		}(i)
		
		// Add to used links with limit
		simpleUsedLinksMu.Lock()
		if len(simpleUsedLinks[ws.SessionString]) < MaxLinksPerSession {
			simpleUsedLinks[ws.SessionString] = append(simpleUsedLinks[ws.SessionString], apiResponse.Results[i].URL)
		}
		simpleUsedLinksMu.Unlock()
	}
	wg.Wait()

	result, err := json.Marshal(summaryResults)

	if err != nil {
		return "", err
	}

	if ws.CallbacksHandler != nil {
		ws.CallbacksHandler.HandleToolEnd(ctx, string(result))
	}

	if len(apiResponse.Results) == 0 {
		return "No results found", fmt.Errorf("No results, we might be rate limited")
	}

	return string(result), nil
}

// CleanupSession removes session data from global maps to prevent memory leaks
func CleanupSimpleWebSearchSession(sessionID string) {
	simpleUsedLinksMu.Lock()
	delete(simpleUsedLinks, sessionID)
	simpleUsedLinksMu.Unlock()
}
