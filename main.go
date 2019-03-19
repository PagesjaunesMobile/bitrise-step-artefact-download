package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const domain = "https://api.bitrise.io"
const apiVersion = "v0.1"

// Client Bitrise API client
type Client struct {
	authToken  string
	httpClient http.Client
}

// Artifacts ...
type Artifacts struct {
	Data []struct {
		ArtifactType        string `json:"artifact_type"`
		IsPublicPageEnabled bool   `json:"is_public_page_enabled"`
		Slug                string `json:"slug"`
		Title               string `json:"title"`
	} `json:"data"`
	Paging struct {
		PageItemLimit  int `json:"page_item_limit"`
		TotalItemCount int `json:"total_item_count"`
	} `json:"paging"`
}

// Artifact ...
type Artifact struct {
	Data struct {
		ArtifactType         string `json:"artifact_type"`
		ExpiringDownloadURL  string `json:"expiring_download_url"`
		IsPublicPageEnabled  bool   `json:"is_public_page_enabled"`
		PublicInstallPageURL string `json:"public_install_page_url"`
		Slug                 string `json:"slug"`
		Title                string `json:"title"`
	} `json:"data"`
}

// New Create new Bitrise API client
func New(authToken string) Client {
	return Client{
		authToken:  authToken,
		httpClient: http.Client{Timeout: 20 * time.Second},
	}
}

func (c Client) get(endpoint string) (*http.Response, error) {
	url := fmt.Sprintf("%s/%s/%s", domain, apiVersion, endpoint)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return &http.Response{}, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("token %s", c.authToken))

	resp, err := c.httpClient.Do(req)
	return resp, err
}

// GetArtifactsForBuild ...
func (c Client) GetArtifactsForBuild(appSlug, buildSlug string) (art Artifacts, err error) {
	requestPath := fmt.Sprintf("apps/%s/builds/%s/artifacts", appSlug, buildSlug)

	resp, err := c.get(requestPath)
	if err != nil {
		return
	}
	defer responseBodyCloser(resp)

	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		err = fmt.Errorf("failed to get artifacts with status code (%d) for [build_slug: %s, app_slug: %s]", resp.StatusCode, appSlug, buildSlug)
		return
	}

	err = json.NewDecoder(resp.Body).Decode(&art)
	return
}

// GetArtifactDetails ...
func (c Client) GetArtifactDetails(appSlug, buildSlug, artifactSlug string) (art Artifact, err error) {
	requestPath := fmt.Sprintf("apps/%s/builds/%s/artifacts/%s", appSlug, buildSlug, artifactSlug)

	resp, err := c.get(requestPath)
	if err != nil {
		return
	}
	defer responseBodyCloser(resp)

	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		err = fmt.Errorf("failed to get artifact details with status code (%d) for [build_slug: %s, app_slug: %s]", resp.StatusCode, appSlug, buildSlug)
		return
	}

	err = json.NewDecoder(resp.Body).Decode(&art)
	return
}

// DownloadArtifact ...
func (c Client) DownloadArtifact(appSlug, buildSlug, artifactSlug string) (io.ReadCloser, error) {
	artifact, err := c.GetArtifactDetails(appSlug, buildSlug, artifactSlug)
	if err != nil {
		return nil, err
	}

	resp, err := http.Get(artifact.Data.ExpiringDownloadURL)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func responseBodyCloser(resp *http.Response) {
	if err := resp.Body.Close(); err != nil {
		log.Printf(" [!] Failed to close response body: %+v", err)
	}
}

func errNoEnv(env string) error {
	return fmt.Errorf("environment variable (%s) is not set", env)
}

func mainE() error {
	accessTokenKey := "API_AUTH_TOKEN"
	accessToken := os.Getenv(accessTokenKey)
	if accessToken == "" {
		return errNoEnv(accessTokenKey)
	}

	appSlugKey := "APP_SLUG"
	appSlug := os.Getenv(appSlugKey)
	if appSlug == "" {
		return errNoEnv(appSlugKey)
	}

	buildSlugKey := "WORKFLOW_SLUG_ID"
	buildSlug := os.Getenv(buildSlugKey)
	if buildSlug == "" {
		return errNoEnv(buildSlugKey)
	}

	artifactNameKey := "ARTIFACT_NAME"
	artifactName := os.Getenv(artifactNameKey)
	if artifactName == "" {
		return errNoEnv(artifactNameKey)
	}

	downloadDirKey := "DOWNLOAD_DIR"
	downloadDir := os.Getenv(downloadDirKey)
	if downloadDir == "" {
		downloadDir = "."
	}

	if err := os.MkdirAll(downloadDir, os.ModePerm); err != nil {
		return err
	}

	c := New(accessToken)
	artifacts, err := c.GetArtifactsForBuild(appSlug, buildSlug)
	if err != nil {
		return err
	}

	artifactSlugMap := map[string]string{}
	for _, artifact := range artifacts.Data {
		artifactSlugMap[artifact.Title] = artifact.Slug
	}

	artifactSlug, exists := artifactSlugMap[artifactName]
	if !exists {
		keys, err := json.MarshalIndent(artifactSlugMap, "", "  ")
		if err != nil {
			return err
		}
		return fmt.Errorf("unable to find artifact with name (%s), available artifacts:\n%s", artifactName, string(keys))
	}

	reader, err := c.DownloadArtifact(appSlug, buildSlug, artifactSlug)
	if err != nil {
		return err
	}

	file, err := os.Create(filepath.Join(downloadDir, artifactName))
	if err != nil {
		return err
	}
	n, err := io.Copy(file, reader)
	if err != nil {
		return err
	}

	fmt.Printf("done, [%d byte] downloaded\n", n)

	return nil
}

func main() {
	if err := mainE(); err != nil {
		fmt.Printf("Error: %+v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}
