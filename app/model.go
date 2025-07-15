package app

import "go.mau.fi/whatsmeow"

type SendMessageRequest struct {
	Client *whatsmeow.Client `json:"client"`
	To     string            `json:"to"`
	Text   string            `json:"text"`
}
