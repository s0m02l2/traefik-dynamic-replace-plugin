package dynamicreplace

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/traefik/traefik/v2/pkg/config/dynamic"
	"github.com/traefik/traefik/v2/pkg/plugins"
)

// Config defines the plugin configuration.
type Config struct {
	APIURL         string   `json:"apiURL,omitempty"`
	ReplaceableKeys []string `json:"replaceableKeys,omitempty"`
	DestinationURL string   `json:"destinationURL,omitempty"`
}

// CreateConfig initializes the plugin configuration.
func CreateConfig() *Config {
	return &Config{}
}

// DynamicReplacePlugin represents the plugin.
type DynamicReplacePlugin struct {
	next             http.Handler
	name             string
	apiURL           string
	replaceableKeys  []string
	destinationURL   string
}

// New creates a new DynamicReplacePlugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if config.APIURL == "" || config.DestinationURL == "" || len(config.ReplaceableKeys) == 0 {
		return nil, fmt.Errorf("invalid configuration")
	}

	return &DynamicReplacePlugin{
		next:             next,
		name:             name,
		apiURL:           config.APIURL,
		replaceableKeys:  config.ReplaceableKeys,
		destinationURL:   config.DestinationURL,
	}, nil
}

// ServeHTTP processes the HTTP request.
func (d *DynamicReplacePlugin) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// Read request body
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		http.Error(rw, "could not read request body", http.StatusInternalServerError)
		return
	}

	// Extract uid from the request body
	var requestData map[string]interface{}
	if err := json.Unmarshal(body, &requestData); err != nil {
		http.Error(rw, "invalid JSON", http.StatusBadRequest)
		return
	}

	uid, ok := requestData["uid"].(string)
	if !ok || uid == "" {
		http.Error(rw, "uid not found in request", http.StatusBadRequest)
		return
	}

	// Fetch additional data using the uid
	fetchedData, err := d.fetchDataFromAPI(uid)
	if err != nil {
		http.Error(rw, "could not fetch data from API", http.StatusInternalServerError)
		return
	}

	// Replace placeholders in the original body
	updatedBody := string(body)
	for _, key := range d.replaceableKeys {
		if value, exists := fetchedData[key]; exists {
			updatedBody = strings.ReplaceAll(updatedBody, fmt.Sprintf("{{%s}}", key), value)
		}
	}

	// Send the updated request to the destination URL
	d.sendToDestination(rw, updatedBody)
}

// fetchDataFromAPI fetches data from the configured API based on the uid.
func (d *DynamicReplacePlugin) fetchDataFromAPI(uid string) (map[string]string, error) {
	apiURL := fmt.Sprintf("%s?uid=%s", d.apiURL, uid)

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status: %s", resp.Status)
	}

	var responseData map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return nil, err
	}

	return responseData, nil
}

// sendToDestination forwards the modified request to the destination URL.
func (d *DynamicReplacePlugin) sendToDestination(rw http.ResponseWriter, updatedBody string) {
	client := &http.Client{}
	req, err := http.NewRequest("POST", d.destinationURL, strings.NewReader(updatedBody))
	if err != nil {
		http.Error(rw, "could not create request to destination", http.StatusInternalServerError)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		http.Error(rw, "could not send request to destination", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	rw.WriteHeader(resp.StatusCode)
	_, _ = rw.Write([]byte(fmt.Sprintf("Request sent to destination with status: %s", resp.Status)))
}

