// Package ai checks uploaded documents with Claude: is this the document we
// asked for, is it legible and valid, and what structured data does it contain.
package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

type Analysis struct {
	DetectedType    string            `json:"detected_type"`
	MatchesExpected bool              `json:"matches_expected"`
	Legible         bool              `json:"legible"`
	Language        string            `json:"language"`
	HasApostille    bool              `json:"has_apostille"`
	ExpiryDate      string            `json:"expiry_date,omitempty"`
	Fields          map[string]string `json:"fields"`
	Issues          []string          `json:"issues"`
	Accept          bool              `json:"accept"`
}

type Client struct {
	c     anthropic.Client
	model string
}

func New(apiKey, model string) *Client {
	return &Client{c: anthropic.NewClient(option.WithAPIKey(apiKey)), model: model}
}

const systemPrompt = `You verify documents uploaded by students applying to universities abroad.
You receive one file and the document type the platform asked for. Decide whether it is that document,
whether it is complete and legible, and extract key fields (full name, date of birth, document number,
issue/expiry dates, institution, grades/GPA, test scores — whichever apply).
Set accept=true only if it matches the expected type, is legible, and nothing blocks its use
(e.g. passport expiring within 12 months, missing pages, cropped photo page).
List every problem in "issues" in plain, short sentences the student can act on.
Respond with a single JSON object and nothing else, with keys:
detected_type, matches_expected, legible, language, has_apostille, expiry_date, fields, issues, accept.`

func (c *Client) AnalyzeDocument(ctx context.Context, expectedType, contentType string, data []byte) (*Analysis, json.RawMessage, error) {
	b64 := base64.StdEncoding.EncodeToString(data)
	var fileBlock anthropic.BetaContentBlockParamUnion
	switch {
	case contentType == "application/pdf":
		fileBlock = anthropic.NewBetaDocumentBlock(anthropic.BetaBase64PDFSourceParam{Data: b64})
	case strings.HasPrefix(contentType, "image/"):
		fileBlock = anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{Data: b64, MediaType: anthropic.BetaBase64ImageSourceMediaType(contentType)})
	default:
		return nil, nil, fmt.Errorf("unsupported content type %q", contentType)
	}

	resp, err := c.c.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 16000,
		System:    []anthropic.BetaTextBlockParam{{Text: systemPrompt}},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(fileBlock, anthropic.NewBetaTextBlock("Expected document type: "+expectedType)),
		},
		// If a safety classifier declines, the request is re-served by a fallback model in the same call.
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
	})
	if err != nil {
		return nil, nil, err
	}
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return nil, nil, errors.New("AI declined to analyse this document")
	}
	var text strings.Builder
	for _, b := range resp.Content {
		if t, ok := b.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	raw := extractJSON(text.String())
	var a Analysis
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, nil, fmt.Errorf("parse AI response: %w", err)
	}
	return &a, raw, nil
}

func extractJSON(s string) json.RawMessage {
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return json.RawMessage("{}")
	}
	return json.RawMessage(s[start : end+1])
}
