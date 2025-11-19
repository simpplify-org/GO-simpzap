package app

import (
	"encoding/json"
	"github.com/labstack/echo/v4"
	"strings"
)

func splitPath(p string) []string {
	p = strings.TrimSpace(p)
	if p == "" {
		return []string{}
	}

	parts := strings.Split(p, "/")
	cleaned := make([]string, 0, len(parts))

	for _, part := range parts {
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}

	return cleaned
}

func extractNumber(c echo.Context, bodyBytes []byte) string {
	path := c.Request().URL.Path

	// ----------------------------
	// 1. Create / Delete -> body
	// ----------------------------
	if path == "/create" || path == "/delete" {
		if len(bodyBytes) > 0 {
			var tmp struct {
				Number string `json:"number"`
			}
			if json.Unmarshal(bodyBytes, &tmp) == nil && tmp.Number != "" {
				return tmp.Number
			}
		}
		return ""
	}

	// ----------------------------
	// 2. Para todas as rotas /device/*
	// ----------------------------
	parts := strings.Split(path, "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "device" && parts[i+1] != "" {
			return parts[i+1]
		}
	}

	return ""
}
