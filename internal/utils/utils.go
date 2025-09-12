package utils

import (
	"encoding/base64"
	"fmt"
	"strings"
)

func GetEncodedBase64Token(username string, password string) string {
	tokenData := fmt.Sprintf("%s:%s", username, password)
	encoded := base64.StdEncoding.EncodeToString([]byte(tokenData))
	return "Basic " + encoded
}

// inspired by private method from http package
func ParseBasicAuth(auth string) (username, password string, ok bool) {
	const prefix = "Basic "
	c, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return "", "", false
	}
	cs := string(c)
	username, password, ok = strings.Cut(cs, ":")
	if !ok {
		return "", "", false
	}
	return username, password, true
}
