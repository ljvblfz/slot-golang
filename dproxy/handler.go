package main

import (
	"net"

	"github.com/golang/glog"
)

type Handler struct {
	Server *Server

	parser   *Parser
	msgQueue chan *Task
}

func NewHandler(workerCount int, httpUrl string) *Handler {
	h := &Handler{
		parser:   NewParser(httpUrl),
		msgQueue: make(chan *Task, 1024*128),
	}
	for i := 0; i < workerCount; i++ {
		go h.processer()
	}
	return h
}

func (h *Handler) Process(peer *net.UDPAddr, msg []byte) {
	msgCopy := make([]byte, len(msg))
	copy(msgCopy, msg)

	t := &Task{
		Peer:   peer,
		Msg:    msgCopy,
		Input:  make(map[string]string),
		Output: make(map[string]string),
	}

	h.msgQueue <- t
}

func (h *Handler) processer() {
	if h.Server == nil {
	}
	for t := range h.msgQueue {
		err := h.parser.Parse(t)
		if err != nil {
			if glog.V(3) {
				glog.Errorf("[handler] parse msg (len[%d] %v) error: %v", len(t.Msg), t.Msg, err)
			} else if glog.V(2) {
				glog.Errorf("[handler] parse msg (len[%d] %v) error: %v", len(t.Msg), t.Msg[:5], err)
			}
			continue
		}

		res := t.Do()
		if len(res) == 0 {
			glog.Errorf("[handler] should not reponse with empty")
		}
		h.Server.Send(t.Peer, res)
	}
}
