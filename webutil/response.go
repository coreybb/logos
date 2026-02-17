package webutil

import (
	"encoding/json"
	"log"
	"net/http"
)

func RespondWithError(w http.ResponseWriter, code int, message string) {
	w.Header().Set(HeaderContentType, ContentTypeJSONUTF8)
	RespondWithJSON(w, code, map[string]string{"error": message})
}

func RespondWithJSON(w http.ResponseWriter, status int, payload any) {
	response, err := json.Marshal(payload)
	if err != nil {
		log.Printf("ERROR: Failed to marshal JSON response: %v", err)
		w.Header().Set(HeaderContentType, ContentTypeJSONUTF8)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal Server Error"}`))
		return
	}

	w.WriteHeader(status)
	_, _ = w.Write(response)
}

func HasResponseWriterSentHeader(w http.ResponseWriter) bool {
	return w.Header().Get(HeaderContentType) != ""
}
