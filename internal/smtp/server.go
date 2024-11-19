package smtp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"strings"
	"sync"
	"time"
)

// MailMessage represents the parsed mail data.
type MailMessage struct {
	From      string
	To        string
	Subject   string
	PlainText string
	HTMLText  string
}

// CallBackFn defines the callback function type for handling parsed messages.
type CallBackFn func(mail *MailMessage) error

// Server represents a basic SMTP server.
type Server struct {
	Address    string
	listener   net.Listener
	CallBackFn CallBackFn

	wg   sync.WaitGroup
	done chan struct{}
}

// NewServer creates a new SMTP server instance.
func NewServer(address string, callback CallBackFn) *Server {
	return &Server{
		Address:    address,
		CallBackFn: callback,

		done: make(chan struct{}, 1),
	}
}

type Session struct {
	from string
	to   string
}

// Start starts the SMTP server and handles incoming connections.
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.Address)
	if err != nil {
		return fmt.Errorf("error starting SMTP server: %w", err)
	}
	s.listener = listener

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				// we called Close()
				return nil
			default:
			}
			if err != nil {
				return fmt.Errorf("connection error: %w", err)
			}
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()

			s.handleConnection(conn)
		}()
	}
}

// Close shuts down the server.
func (s *Server) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *Server) Shutdown() error {
	select {
	case <-s.done:
		return errors.New("server already closed")
	default:
		close(s.done)
	}

	s.wg.Wait()

	return s.Close()
}

// handleConnection processes an SMTP client connection.
func (s *Server) handleConnection(conn net.Conn) {
	var (
		from string
		to   string
	)

	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	conn.SetDeadline(time.Now().Add(30 * time.Second))
	writer.WriteString("220 Welcome to the SMTP server\r\n")
	writer.Flush()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(line, "EHLO"):
			// Handle EHLO command
			s.handleEHLO(writer)

		case strings.HasPrefix(line, "MAIL FROM"):
			// Handle MAIL FROM command
			if from, err = s.parseAddress(line); err != nil {
				_, _ = writer.WriteString(fmt.Sprintf("550 Error: %v\r\n", err))
				_ = writer.Flush()

				return
			}
			_, _ = writer.WriteString("250 OK\r\n")
			if err := writer.Flush(); err != nil {
				return
			}
		case strings.HasPrefix(line, "RCPT TO"):
			if to, err = s.parseAddress(line); err != nil {
				_, _ = writer.WriteString(fmt.Sprintf("550 Error: %v\r\n", err))
				_ = writer.Flush()

				return
			}
			_, _ = writer.WriteString("250 OK\r\n")
			if err := writer.Flush(); err != nil {
				return
			}
		case strings.HasPrefix(line, "DATA"):
			_, _ = writer.WriteString("354 Start mail input; end with <CRLF>.<CRLF>\r\n")
			if err := writer.Flush(); err != nil {
				return
			}

			mailData := collectMailData(reader)
			if mailData == "" {
				_, _ = writer.WriteString("550 Error reading mail data\r\n")
				_ = writer.Flush()
				return
			}

			msg, err := parseMailData(mailData)
			if err != nil {
				_, _ = writer.WriteString(fmt.Sprintf("550 Error processing mail: %v\r\n", err))
				_ = writer.Flush()
				return
			}

			if msg.From != from {
				msg.From = from
			}

			if msg.To != to {
				msg.To = to
			}

			// Invoke the callback function
			if err := s.CallBackFn(msg); err != nil {
				_, _ = writer.WriteString(fmt.Sprintf("550 Error processing mail: %v\r\n", err))
				_ = writer.Flush()
				return
			}

			_, _ = writer.WriteString("250 OK\r\n")
			if err := writer.Flush(); err != nil {
				return
			}

		case strings.HasPrefix(line, "QUIT"):
			_, _ = writer.WriteString("221 Bye\r\n")
			_ = writer.Flush()
			return // Close connection after QUIT command

		case strings.HasPrefix(line, "NOOP"):
			_, _ = writer.WriteString("250 OK\r\n")
			if err := writer.Flush(); err != nil {
				return
			}

		case line == ".":
			// End of data
			break

		default:
			_, _ = writer.WriteString("250 OK\r\n")
			if err := writer.Flush(); err != nil {
				return
			}
		}
	}
}

// collectMailData reads the raw mail data from the client until the SMTP end marker (".").
func collectMailData(reader *bufio.Reader) string {
	var mailData strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return ""
		}
		if strings.TrimSpace(line) == "." {
			break
		}
		mailData.WriteString(line)
	}
	return strings.TrimSpace(mailData.String())
}

// parseMailData parses the raw mail data into a MailMessage struct.
func parseMailData(data string) (*MailMessage, error) {
	headers, body, err := parseHeadersAndBody(data)
	if err != nil {
		return nil, err
	}

	// Case-insensitive header lookup
	getHeader := func(key string) string {
		for k, v := range headers {
			if strings.EqualFold(k, key) {
				return v
			}
		}
		return ""
	}

	from := getHeader("From")
	to := getHeader("To")
	subject := getHeader("Subject")

	contentType := getHeader("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil && contentType != "" {
		return nil, fmt.Errorf("error parsing Content-Type: %w", err)
	}

	mailMessage := &MailMessage{
		From:    from,
		To:      to,
		Subject: subject,
	}

	// Handle multipart messages
	if strings.HasPrefix(mediaType, "multipart/") {
		err = processMultipartMessage(strings.NewReader(body), params["boundary"], mailMessage)
		if err != nil {
			return nil, err
		}
	} else {
		// Handle simple message (non-multipart)
		mailMessage.PlainText = body
	}

	return mailMessage, nil
}

// parseHeadersAndBody splits raw mail data into headers and body.
func parseHeadersAndBody(data string) (map[string]string, string, error) {
	parts := strings.SplitN(data, "\r\n\r\n", 2)
	if len(parts) < 2 {
		return nil, "", errors.New("invalid mail format: missing headers or body")
	}

	headers := make(map[string]string)
	for _, line := range strings.Split(parts[0], "\r\n") {
		colonIndex := strings.Index(line, ":")
		if colonIndex == -1 {
			return nil, "", fmt.Errorf("invalid header format: %s", line)
		}
		key := strings.TrimSpace(line[:colonIndex])
		value := strings.TrimSpace(line[colonIndex+1:])
		headers[key] = value
	}

	body := parts[1]
	return headers, body, nil
}

// processMultipartMessage processes a multipart message and populates the MailMessage fields.
func processMultipartMessage(bodyReader io.Reader, boundary string, mailMessage *MailMessage) error {
	multipartReader := multipart.NewReader(bodyReader, boundary)

	for {
		part, err := multipartReader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading multipart message: %w", err)
		}
		defer part.Close()

		// Process each part
		partContentType := part.Header.Get("Content-Type")
		partData, err := io.ReadAll(part)
		if err != nil {
			return fmt.Errorf("error reading part: %w", err)
		}

		if strings.HasPrefix(partContentType, "text/plain") {
			mailMessage.PlainText = strings.TrimSpace(string(partData))
		} else if strings.HasPrefix(partContentType, "text/html") {
			mailMessage.HTMLText = strings.TrimSpace(string(partData))
		}
	}

	return nil
}

// handleEHLO handles the EHLO command and sends a list of supported extensions.
func (s *Server) handleEHLO(writer *bufio.Writer) {
	// List of supported SMTP extensions (as per your server configuration)
	extensions := []string{
		"250-SIZE 10240000", // 10 MB message size limit
	}

	// Send the EHLO response
	writer.WriteString("250-Hello\r\n") // "250" is the response code for a successful command
	for _, ext := range extensions {
		writer.WriteString(ext + "\r\n")
	}
	writer.WriteString("250 OK\r\n") // End of EHLO response
	_ = writer.Flush()
}

// parseAddress handles the MAIL FROM AND RCPT TO command.
func (s *Server) parseAddress(line string) (string, error) {
	// Extract recipient's email address from the command
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return "", errors.New("invalid RCPT TO syntax")
	}

	address := strings.TrimSpace(parts[1])
	return strings.Trim(address, "<>"), nil
}
