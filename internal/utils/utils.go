package utils

import (
	"encoding/base64"
	"fmt"
)

func GetEncodedBase64Token(username string, password string) string {
	tokenData := fmt.Sprintf("%s:%s", username, password)
	encoded := base64.StdEncoding.EncodeToString([]byte(tokenData))
	return "Basic " + encoded
}
