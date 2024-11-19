package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/cloudeteer/azure-communication-gateway-smtp-bridge/email"
	"github.com/cloudeteer/azure-communication-gateway-smtp-bridge/smtp"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error(err.Error())

		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	cred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: httpClient,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get default credential: %w", err)
	}

	ctx := context.Background()

	_, err = cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://communication.azure.com/.default"},
	})
	if err != nil {
		return fmt.Errorf("failed to get token from Azure: %w", err)
	}

	connectionString, ok := os.LookupEnv("COMMUNICATION_SERVICES_CONNECTION_STRING")
	if !ok {
		return fmt.Errorf("COMMUNICATION_SERVICES_CONNECTION_STRING is not set")
	}

	emailClient := email.NewClient(connectionString, httpClient, cred)

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	logger.Info("Starting SMTP server at port 1025")

	errCh := make(chan error, 1)

	server := smtp.NewServer(":2525", func(mail *smtp.MailMessage) error {
		return emailClient.SendEmail(context.Background(), &email.Email{
			SenderAddress: mail.From,
			Recipients: email.Recipients{
				To: []email.EmailAddress{
					{mail.To, mail.To},
				},
			},
			Content: email.Content{
				Subject:   mail.Subject,
				PlainText: mail.PlainText,
				HTML:      mail.HTMLText,
			},
		})
	})

	go func() {
		if err := server.Start(); err != nil {
			fmt.Printf("Error: %v\n", err)
		}

		close(errCh)
	}()

	select {
	case <-ctx.Done():
		logger.Info("Shutting down SMTP server")

		return server.Shutdown()
	case err := <-errCh:
		return err
	}
}
