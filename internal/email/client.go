package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

func NewClient(endpoint string, httpClient *http.Client, tokenCredential azcore.TokenCredential) *Client {
	return &Client{
		endpoint:        endpoint,
		tokenCredential: tokenCredential,
		httpClient:      httpClient,
	}
}

func (client *Client) SendEmail(ctx context.Context, email *Email) error {
	postBody, err := json.Marshal(email)
	if err != nil {
		return fmt.Errorf("failed to marshal email: %w", err)
	}

	url := client.endpoint + "/emails:send?api-version=2023-03-31"
	bodyBuffer := bytes.NewBuffer(postBody)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bodyBuffer)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	token, err := client.tokenCredential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://communication.azure.com/.default"},
	})
	if err != nil {
		return fmt.Errorf("failed to get token: %w", err)
	}

	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
		commError := ErrorResponse{}

		err = json.NewDecoder(resp.Body).Decode(&commError)
		if err != nil {
			return fmt.Errorf("failed to decode error response: %w", err)
		}

		return fmt.Errorf("status code: %d, error message: %s", resp.StatusCode, commError.Error.Message)
	}

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("status code: %d", resp.StatusCode)
	}

	return nil
}
