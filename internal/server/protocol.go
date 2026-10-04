package server

import "encoding/json"

// ProtocolVersion is sent in the hello message. See docs/SPEC.md §3.
const ProtocolVersion = 1

// Control messages are JSON objects in websocket text frames; terminal bytes
// travel in binary frames.

// ClientMessage is any client→server control message.
type ClientMessage struct {
	Type   string `json:"type"`
	Cols   int    `json:"cols,omitempty"`
	Rows   int    `json:"rows,omitempty"`
	Redraw bool   `json:"redraw,omitempty"`
}

type helloMessage struct {
	Type    string `json:"type"` // "hello"
	Version int    `json:"version"`
	ID      string `json:"id"`
	Name    string `json:"name"`
}

type syncMessage struct {
	Type  string `json:"type"`  // "sync"
	State string `json:"state"` // "start" or "end"
}

type exitMessage struct {
	Type string `json:"type"` // "exit"
	Code int    `json:"code"`
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
