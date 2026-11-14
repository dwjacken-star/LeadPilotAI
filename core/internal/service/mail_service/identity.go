package mail_service

import (
	"billionmail-core/internal/service/deliveryprefs"
	"context"
	"errors"
	"net/textproto"
	"strings"
)

var SuppressionCheck = deliveryprefs.IsSuppressed

func (e *EmailSender) checkSuppression(message Message, recipients []string) error {
	if message.transactional {
		return nil
	}
	identity, err := OutboundIdentity(e.Email)
	if err != nil {
		return err
	}
	for _, recipient := range recipients {
		blocked, err := SuppressionCheck(context.Background(), identity.From, recipient)
		if err != nil {
			return errors.New("unsubscribe protection unavailable; message not sent")
		}
		if blocked {
			return errors.New("recipient has unsubscribed; message not sent")
		}
	}
	return nil
}

type DeliveryIdentity struct {
	From    string `json:"from"`
	ReplyTo string `json:"reply_to"`
}

// Authenticated senders and SMTP envelope senders remain unchanged for permissions,
// rotation and bounce processing. These addresses describe the customer-visible identity.
func OutboundIdentity(fallback string) (DeliveryIdentity, error) {
	identity := configuredIdentity()
	if e := validateIdentity(identity); e != nil {
		return DeliveryIdentity{}, e
	}
	if identity.From == "" {
		identity.From = fallback
	}
	return identity, nil
}

func prepareDeliveryIdentity(message Message, sender string) (Message, string, error) {
	headers := make(map[string]string, len(message.Headers)+1)
	for key, value := range message.Headers {
		if strings.ContainsAny(key, ":\r\n") || strings.ContainsAny(value, "\r\n") {
			return message, "", errors.New("invalid email header")
		}
		canonical := mailHeaderKey(key)
		if _, exists := headers[canonical]; exists {
			return message, "", errors.New("duplicate email header")
		}
		headers[canonical] = value
	}
	message.Headers = headers
	identity, e := OutboundIdentity(sender)
	if e != nil {
		return message, "", e
	}
	from := sender
	if custom := headers["From"]; custom != "" {
		from = custom
	}
	if configuredIdentity().From != "" {
		from = identity.From
	}
	if name := headers["Realname"]; name != "" {
		if identity.From != sender || headers["From"] == "" {
			from = name + " <" + identity.From + ">"
		}
	}
	delete(headers, "Realname")
	delete(headers, "From")
	if identity.ReplyTo != "" {
		headers["Reply-To"] = identity.ReplyTo
	}
	return message, from, nil
}

func mailHeaderKey(key string) string {
	// MIME canonicalization alone turns Message-ID into Message-Id, while the
	// existing builder expects the established names below.
	switch strings.ToLower(key) {
	case "message-id":
		return "Message-ID"
	case "reply-to":
		return "Reply-To"
	case "content-type":
		return "Content-Type"
	case "content-transfer-encoding":
		return "Content-Transfer-Encoding"
	case "from":
		return "From"
	case "to":
		return "To"
	case "realname":
		return "Realname"
	case "date":
		return "Date"
	}
	return textproto.CanonicalMIMEHeaderKey(key)
}
