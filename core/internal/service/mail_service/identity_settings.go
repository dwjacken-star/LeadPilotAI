package mail_service

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"os"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/frame/g"
)

const identityOption = "outbound_identity"

var identityConfig struct {
	sync.RWMutex
	value *DeliveryIdentity
}

func configuredIdentity() DeliveryIdentity {
	identityConfig.RLock()
	defer identityConfig.RUnlock()
	if identityConfig.value != nil {
		return *identityConfig.value
	}
	// Bootstrap compatibility only. Production initializes the persistent option
	// before workers start; subsequent environment changes cannot override it.
	return DeliveryIdentity{From: strings.TrimSpace(os.Getenv("MAPLEMAIL_FROM_ADDRESS")), ReplyTo: strings.TrimSpace(os.Getenv("MAPLEMAIL_REPLY_TO_ADDRESS"))}
}

func validateIdentity(identity DeliveryIdentity) error {
	for _, value := range []string{identity.From, identity.ReplyTo} {
		if value == "" {
			continue
		}
		parsed, e := mail.ParseAddress(value)
		if e != nil || strings.ContainsAny(value, "\r\n") || parsed.Address != value {
			return errors.New("invalid outbound email identity")
		}
	}
	return nil
}

func InitializeOutboundIdentity(ctx context.Context) error {
	value, e := g.DB().Model("bm_options").Ctx(ctx).Where("name", identityOption).Value("value")
	if e != nil {
		return e
	}
	var identity DeliveryIdentity
	if value.IsEmpty() {
		identity = configuredIdentity()
		if e = validateIdentity(identity); e != nil {
			return e
		}
		encoded, _ := json.Marshal(identity)
		if _, e = g.DB().Model("bm_options").Ctx(ctx).Data(g.Map{"name": identityOption, "value": string(encoded)}).OnConflict("name").InsertIgnore(); e != nil {
			return e
		}
		value, e = g.DB().Model("bm_options").Ctx(ctx).Where("name", identityOption).Value("value")
		if e != nil {
			return e
		}
	}
	if e = json.Unmarshal(value.Bytes(), &identity); e != nil {
		return errors.New("invalid stored outbound identity")
	}
	if e = validateIdentity(identity); e != nil {
		return e
	}
	identityConfig.Lock()
	identityConfig.value = &identity
	identityConfig.Unlock()
	return nil
}

func SaveOutboundIdentity(ctx context.Context, identity DeliveryIdentity) error {
	identity.From = strings.TrimSpace(identity.From)
	identity.ReplyTo = strings.TrimSpace(identity.ReplyTo)
	if e := validateIdentity(identity); e != nil {
		return e
	}
	if identity.From != "" {
		n, e := g.DB().Model("mailbox").Ctx(ctx).Where("username", identity.From).Where("active", 1).Count()
		if e != nil {
			return e
		}
		if n == 0 {
			return errors.New("From must be an active local mailbox")
		}
	}
	encoded, _ := json.Marshal(identity)
	_, e := g.DB().Model("bm_options").Ctx(ctx).Data(g.Map{"name": identityOption, "value": string(encoded)}).OnConflict("name").Save()
	if e != nil {
		return e
	}
	identityConfig.Lock()
	identityConfig.value = &identity
	identityConfig.Unlock()
	return nil
}
