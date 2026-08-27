package httpapi

import (
	"encoding/json"
	"net/http"

	"task289-compverify/internal/model"
)

// writeJSON 以 JSON 写出响应体。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr 以稳定错误码写出错误响应。
func writeErr(w http.ResponseWriter, status int, err error) {
	code := "INTERNAL"
	if de, ok := err.(*model.DomainError); ok {
		code = de.Code
	}
	writeJSON(w, status, map[string]string{"error": code, "message": err.Error()})
}

// statusFor 把领域错误映射为 HTTP 状态码。
func statusFor(err error) int {
	if de, ok := err.(*model.DomainError); ok {
		switch de.Code {
		case "NOT_FOUND":
			return http.StatusNotFound
		case "BAD_INPUT":
			return http.StatusBadRequest
		case "CONFLICT", "DUPLICATE_EVENT", "ILLEGAL_TRANSITION", "GEN_REGRESS", "SELF_LOOP", "UNKNOWN_EFFECT":
			return http.StatusConflict
		case "SEALED":
			return http.StatusForbidden
		}
	}
	return http.StatusInternalServerError
}

// decodeJSON 解码请求体。
func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	return dec.Decode(dst)
}

// pathID 从路径参数解析正整数 ID。
func pathID(r *http.Request, name string) (int64, bool) {
	v := r.PathValue(name)
	id, err := parseInt64(v)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func parseInt64(s string) (int64, error) {
	if s == "" {
		return 0, errEmptyPath
	}
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errBadPath
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

// 包内哨兵错误。
type pathError struct{ msg string }

func (e *pathError) Error() string { return e.msg }

var errEmptyPath = &pathError{msg: "empty path segment"}
var errBadPath = &pathError{msg: "non-numeric path segment"}
