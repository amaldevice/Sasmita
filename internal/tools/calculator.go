package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// calculator evaluates arithmetic only. It is a hand-written precedence
// parser: no JavaScript, shell or template evaluation ever reaches a model.
type calculator struct{}

func (calculator) Spec() Spec {
	return Spec{
		Name: "calculator", Description: "Evaluate a safe arithmetic expression (+ - * / and parentheses).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string"}},"required":["expression"]}`),
		Risk:        RiskCompute, Timeout: defaultTimeout, MaxOutputBytes: 1024,
	}
}

func (calculator) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Expression string `json:"expression"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	value, err := evalArithmetic(in.Expression)
	if err != nil {
		return "", err
	}
	return strconv.FormatFloat(value, 'f', -1, 64), nil
}

func evalArithmetic(input string) (float64, error) {
	if len(input) > 1024 {
		return 0, fmt.Errorf("expression too long")
	}
	p := &exprParser{src: input}
	value, err := p.expression(0)
	if err != nil {
		return 0, err
	}
	if p.pos != len(p.src) {
		return 0, fmt.Errorf("unexpected input at offset %d", p.pos)
	}
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return 0, fmt.Errorf("result is not finite")
	}
	return value, nil
}

type token struct {
	text string
	pos  int
}

type exprParser struct {
	src  string
	pos  int
	next *token
}

func (p *exprParser) peek() token {
	if p.next != nil {
		return *p.next
	}
	i := p.pos
	for i < len(p.src) && unicode.IsSpace(rune(p.src[i])) {
		i++
	}
	if i == len(p.src) {
		return token{pos: i}
	}
	start := i
	switch {
	case strings.ContainsRune("+-*/()", rune(p.src[i])):
		i++
	default:
		for i < len(p.src) && (p.src[i] >= '0' && p.src[i] <= '9' || p.src[i] == '.') {
			i++
		}
		if i == start {
			return token{text: string(p.src[start]), pos: start}
		}
	}
	p.pos = start
	return token{text: p.src[start:i], pos: start}
}

func (p *exprParser) take() token {
	t := p.peek()
	p.next = nil
	if t.pos >= len(p.src) {
		return t
	}
	p.pos = t.pos + len(t.text)
	return t
}

func (p *exprParser) expression(depth int) (float64, error) {
	if depth > 32 {
		return 0, fmt.Errorf("expression nested too deeply")
	}
	value, err := p.term(depth)
	if err != nil {
		return 0, err
	}
	for {
		op := p.peek().text
		if op != "+" && op != "-" {
			return value, nil
		}
		p.take()
		rhs, err := p.term(depth)
		if err != nil {
			return 0, err
		}
		if op == "+" {
			value += rhs
		} else {
			value -= rhs
		}
	}
}

func (p *exprParser) term(depth int) (float64, error) {
	value, err := p.factor(depth)
	if err != nil {
		return 0, err
	}
	for {
		op := p.peek().text
		if op != "*" && op != "/" {
			return value, nil
		}
		p.take()
		rhs, err := p.factor(depth)
		if err != nil {
			return 0, err
		}
		if op == "*" {
			value *= rhs
			continue
		}
		if rhs == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		value /= rhs
	}
}

func (p *exprParser) factor(depth int) (float64, error) {
	t := p.take()
	if t.text == "" {
		return 0, fmt.Errorf("missing operand")
	}
	switch t.text {
	case "+":
		return p.factor(depth)
	case "-":
		v, err := p.factor(depth)
		return -v, err
	case "(":
		v, err := p.expression(depth + 1)
		if err != nil {
			return 0, err
		}
		if p.take().text != ")" {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		return v, nil
	case ")", "*", "/":
		return 0, fmt.Errorf("unexpected %q", t.text)
	}
	v, err := strconv.ParseFloat(t.text, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", t.text)
	}
	return v, nil
}
