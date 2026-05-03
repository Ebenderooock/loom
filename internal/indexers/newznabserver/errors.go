package newznabserver

import (
	"encoding/xml"
	"fmt"
	"net/http"
)

// Newznab error codes recognised by Sonarr, Radarr, and friends.
// Source: https://github.com/Sonarr/Sonarr/wiki/Indexers and the
// upstream Newznab spec. We only define the codes we actually emit;
// passing through unknown codes is fine, but keeping the list short
// makes the package easy to audit.
const (
	errCodeIncorrectParams  = 200
	errCodeMissingParameter = 200
	errCodeAPIKeyMissing    = 100
	errCodeAPIKeyInvalid    = 101
	errCodeAPIKeyDisabled   = 102
	errCodeFunctionNotImpl  = 202
	errCodeInternal         = 900
)

// errorXML is the wire shape Newznab clients expect for errors. The
// element is unnamespaced; Sonarr surfaces `description` verbatim in
// its UI so we keep the text human-readable.
type errorXML struct {
	XMLName     xml.Name `xml:"error"`
	Code        int      `xml:"code,attr"`
	Description string   `xml:"description,attr"`
}

// writeError renders a Newznab-shaped error document. We always use a
// 200-style envelope status (Newznab errors live inside the body) so
// HTTP middleware doesn't replace the body with a generic page; the
// caller chooses the actual status code via httpStatus.
func writeError(w http.ResponseWriter, httpStatus, code int, description string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(httpStatus)
	doc := errorXML{Code: code, Description: description}
	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		// xml.Marshal on a struct with two string attrs cannot
		// realistically fail; fall back to a hand-rolled body so
		// the client still gets a parseable document.
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>`+"\n"+
			`<error code="%d" description="%s"/>`+"\n", code, description)
		return
	}
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(body)
}
