package main

import (
	"fmt"
	"strings"
)

type Parser struct {
	kApiUrls []string
}

func NewParser(httpUrl string) *Parser {
	urls := make([]string, CmdMax)
	urls[CmdRegister] = httpUrl + UrlRegister
	urls[CmdLogin] = httpUrl + UrlLogin
	urls[CmdBind] = httpUrl + UrlBind
	return &Parser{kApiUrls: urls}
}

func (p *Parser) Parse(t *Task) error {
	if len(t.Msg) == 0 {
		return fmt.Errorf("message should not be empty")
	}
	// TODO parse udp header

	// TODO decode data
	c := t.Msg[0]
	switch c {
	case CmdRegister:
	case CmdLogin:
	case CmdBind:
	default:
		return fmt.Errorf("invalid command type %v", t.Msg[0])
	}
	t.CmdType = int8(c)
	t.Url = p.kApiUrls[c]

	inputs := strings.Split(strings.TrimSpace(string(t.Msg[1:])), "&")
	for _, in := range inputs {
		args := strings.SplitN(in, "=", 2)
		if len(args) == 0 {
			return fmt.Errorf("empty argument [%v]", in)
		}
		if len(args[0]) == 0 {
			return fmt.Errorf("key in form cannot be empty [%v]", in)
		}
		if len(args) > 1 {
			t.Input[args[0]] = args[1]
		} else {
			t.Input[args[0]] = ""
		}
	}

	return nil
}
