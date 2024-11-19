package smtp_test

import (
	"fmt"
	"net/smtp"
	"strings"
	"testing"

	smtpserver "github.com/cloudeteer/azure-communication-gateway-smtp-bridge/internal/smtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name              string
		from              string
		to                string
		subject           string
		body              string
		expectedPlainText string
		expectedHTMLText  string
	}{
		{
			name:              "test server simple body",
			from:              "from@example.com",
			to:                "to@example.com",
			subject:           "test subject",
			body:              "test body",
			expectedPlainText: "test body",
			expectedHTMLText:  "",
		},
		{
			name:    "test server multi-part body",
			from:    "from@example.com",
			to:      "to@example.com",
			subject: "test subject",
			body: `MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="boundary42"

--boundary42
Content-Type: text/plain; charset="UTF-8"

Hi there,
This is a plain text version of the email.
Best regards,
Your Name

--boundary42
Content-Type: text/html; charset="UTF-8"

<html>
  <body>
    <p>Hi there,<br>
       This is an <b>HTML</b> version of the email.<br>
       Best regards,<br>
       Your Name
    </p>
  </body>
</html>

--boundary42--`,
			expectedPlainText: `Hi there,
This is a plain text version of the email.
Best regards,
Your Name`,
			expectedHTMLText: `<html>
  <body>
    <p>Hi there,<br>
       This is an <b>HTML</b> version of the email.<br>
       Best regards,<br>
       Your Name
    </p>
  </body>
</html>`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {

			// Create a new server
			s := smtpserver.NewServer("localhost:1515", func(mail *smtpserver.MailMessage) error {
				assert.Equal(t, test.expectedPlainText, strings.ReplaceAll(mail.PlainText, "\r\n", "\n"))
				assert.Equal(t, test.expectedHTMLText, strings.ReplaceAll(mail.HTMLText, "\r\n", "\n"))
				assert.Equal(t, test.subject, mail.Subject)
				assert.Equal(t, test.from, mail.From)
				assert.Equal(t, test.to, mail.To)

				return nil
			})

			errCh := make(chan error, 1)

			go func() {
				errCh <- s.Start()
				close(errCh)
			}()

			body := test.body

			if !strings.Contains(body, ":") {
				body = "\r\n" + body
			}

			msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\n"+
				"Subject: %s\r\n"+
				"%s\r\n", test.from, test.to, test.subject, test.body))

			err := smtp.SendMail("localhost:1515", nil, test.from, []string{test.to}, msg)
			require.NoError(t, err)

			require.NoError(t, s.Shutdown())
			require.NoError(t, <-errCh)
		})
	}
}
