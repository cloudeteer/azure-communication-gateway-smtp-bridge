package email

import (
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

type Client struct {
	endpoint        string
	httpClient      *http.Client
	tokenCredential azcore.TokenCredential
}

type Email struct {
	Recipients    Recipients `json:"recipients"`
	SenderAddress string     `json:"senderAddress"`
	Content       Content    `json:"content"`
	Tracking      bool       `json:"disableUserEngagementTracking"`
	Importance    string     `json:"importance"`
	ReplyTo       []Address  `json:"replyTo"`
}

type Recipients struct {
	To  []Address `json:"to"`
	CC  []Address `json:"cc"`
	BCC []Address `json:"bcc"`
}

type Address struct {
	DisplayName string `json:"displayName"`
	Address     string `json:"address"`
}

type Content struct {
	Subject   string `json:"subject"`
	HTML      string `json:"html"`
	PlainText string `json:"plainText"`
}

type ErrorResponse struct {
	Error CommunicationError `json:"error"`
}

// CommunicationError contains the error code and message.
type CommunicationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
