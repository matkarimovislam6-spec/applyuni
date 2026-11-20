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

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/verify"
)

// Analysis is the AI's reading of one file; the decision is made by the
// rules in package verify, not here.
type Analysis = verify.Extraction

type Client struct {
	c     anthropic.Client
	model string
}

func New(apiKey, model string) *Client {
	return &Client{c: anthropic.NewClient(option.WithAPIKey(apiKey)), model: model}
}

const systemPrompt = `You read documents uploaded by students applying to universities abroad.
You receive one file and the document type the platform asked for. Report what you see; you do not decide
admission. Be literal: copy names exactly as printed (keep the original script), never guess unreadable values,
leave a field empty if it isn't on the document.

Respond with a single JSON object and nothing else, with keys:
- detected_type: what this document actually is, as a short snake_case type (e.g. passport, id_card, school_diploma, bank_statement, unknown)
- matches_expected: true if it is the requested document type
- legible: false if any part needed to read the requested fields is blurred, cut off, covered or too dark
- confidence: 0 to 1, how sure you are about the fields you extracted
- language: ISO 639-1 code of the document's main language (uz, ru, en, it, de, ...)
- has_apostille: true if an apostille stamp or certificate is attached
- fields: an object with exactly the requested field keys; dates as YYYY-MM-DD
- issues: short sentences, addressed to the student, for anything that would stop a university accepting it
- accept: true if nothing you can see would stop a university from accepting it`

func (c *Client) AnalyzeDocument(ctx context.Context, expectedType, contentType string, data []byte) (*Analysis, json.RawMessage, error) {
	spec := verify.SpecFor(expectedType)
	request := fmt.Sprintf("Requested document type: %s\nWhat it should be: %s\nFields to extract: %s",
		expectedType, orDash(spec.Hint), orDash(strings.Join(spec.Fields, ", ")))
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
			anthropic.NewBetaUserMessage(fileBlock, anthropic.NewBetaTextBlock(request)),
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

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func extractJSON(s string) json.RawMessage {
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return json.RawMessage("{}")
	}
	return json.RawMessage(s[start : end+1])
}
