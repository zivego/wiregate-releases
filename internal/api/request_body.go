package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

type requestBodyError struct {
	status  int
	code    string
	message string
}

func (e requestBodyError) Error() string {
	return e.message
}

func decodeJSONBodyWithLimit(w http.ResponseWriter, req *http.Request, dst any, maxBytes int64) error {
	if !isJSONContentType(req.Header.Get("Content-Type")) {
		return requestBodyError{
			status:  http.StatusUnsupportedMediaType,
			code:    "unsupported_media_type",
			message: "Content-Type must be application/json",
		}
	}

	if maxBytes > 0 {
		req.Body = http.MaxBytesReader(w, req.Body, maxBytes)
	}

	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return mapJSONDecodeError(err)
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return mapJSONDecodeError(err)
	}
	return requestBodyError{
		status:  http.StatusBadRequest,
		code:    "validation_failed",
		message: "invalid request body",
	}
}

func isJSONContentType(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return false
	}
	return mediaType == "application/json"
}

func mapJSONDecodeError(err error) error {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return requestBodyError{
			status:  http.StatusRequestEntityTooLarge,
			code:    "payload_too_large",
			message: "request body exceeds size limit",
		}
	}
	return requestBodyError{
		status:  http.StatusBadRequest,
		code:    "validation_failed",
		message: "invalid request body",
	}
}

func (r *Router) decodeJSONBody(w http.ResponseWriter, req *http.Request, dst any) error {
	return decodeJSONBodyWithLimit(w, req, dst, r.maxJSONBytes)
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var decodeErr requestBodyError
	if errors.As(err, &decodeErr) {
		writeError(w, decodeErr.status, decodeErr.code, decodeErr.message)
		return
	}
	writeError(w, http.StatusBadRequest, "validation_failed", "invalid request body")
}
