package crm

import (
	"encoding/base64"
	"golang.org/x/net/html/charset"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
)

func decodeHeader(value string) string {
	decoder := mime.WordDecoder{CharsetReader: charset.NewReaderLabel}
	decoded, err := decoder.DecodeHeader(value)
	if err == nil {
		return decoded
	}
	return value
}
func parseBody(header mail.Header, body io.Reader, depth int) (html, text string) {
	if depth > 10 {
		return "", ""
	}
	kind, params, _ := mime.ParseMediaType(header.Get("Content-Type"))
	disposition, _, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
	if disposition == "attachment" {
		return "", ""
	}
	if strings.HasPrefix(kind, "multipart/") {
		reader := multipart.NewReader(body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err != nil {
				break
			}
			h, t := parseBody(mail.Header(part.Header), part, depth+1)
			html += h
			text += t
		}
		return
	}
	switch strings.ToLower(header.Get("Content-Transfer-Encoding")) {
	case "base64":
		body = base64.NewDecoder(base64.StdEncoding, body)
	case "quoted-printable":
		body = quotedprintable.NewReader(body)
	}
	if label := params["charset"]; label != "" {
		if converted, err := charset.NewReaderLabel(label, body); err == nil {
			body = converted
		}
	}
	raw, _ := io.ReadAll(io.LimitReader(body, 2*1024*1024))
	if kind == "text/html" {
		return string(raw), ""
	}
	if kind == "text/plain" || kind == "" {
		return "", string(raw)
	}
	return "", ""
}
