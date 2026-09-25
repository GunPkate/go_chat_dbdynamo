// This file implements the 5 AI-driven features using a locally-running
// Qwen model via Ollama — no API key, no external account, no cost.
// Good enough for a POC; swap callQwen for a hosted-model call later if
// you need stronger reasoning or don't want to run inference locally.
package logic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// --- Deterministic price-constraint enforcement ---
//
// LLMs — especially small local ones like Qwen 3B — don't reliably honor
// numeric constraints ("under $10") from prose instructions alone. Rather
// than hope the model filters correctly, we parse the constraint in Go
// and enforce it twice: once by filtering the catalog *before* it's sent
// as context (so the model can't even see over-budget items), and once
// again on the returned products afterward (in case it names an ID from
// memory that isn't in the filtered list).

var maxPricePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:under|below|less than|cheaper than|not over|not more than|max(?:imum)?|up to|no more than)\s*\$?\s*(\d+(?:\.\d+)?)`),
	regexp.MustCompile(`(?i)\$?\s*(\d+(?:\.\d+)?)\s*(?:dollars?)?\s*or less`),
	regexp.MustCompile(`(?i)\$?\s*(\d+(?:\.\d+)?)\s*(?:dollars?)?\s*(?:or lower|max(?:imum)?)`),
}

// extractMaxPrice looks for common natural-language budget phrasings
// ("under $10", "less than 15 dollars", "$8 or less", ...) and returns
// the numeric ceiling if found.
func extractMaxPrice(text string) *float64 {
	for _, re := range maxPricePatterns {
		if m := re.FindStringSubmatch(text); m != nil {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				return &v
			}
		}
	}
	return nil
}

func filterByMaxPrice(products []Product, maxPrice *float64) []Product {
	if maxPrice == nil {
		return products
	}
	filtered := make([]Product, 0, len(products))
	for _, p := range products {
		if p.Price <= *maxPrice {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func priceConstraintNote(maxPrice *float64) string {
	if maxPrice == nil {
		return ""
	}
	return fmt.Sprintf("\nThe catalog below has already been filtered to the guest's budget of $%.2f or less — every item shown is a valid option on price; don't second-guess or re-check prices yourself.", *maxPrice)
}

func ollamaURL() string {
	host := envOr("OLLAMA_HOST", "http://localhost:11434")
	return strings.TrimSuffix(host, "/") + "/api/chat"
}

func qwenModel() string {
	return envOr("QWEN_MODEL", "qwen2.5:3b")
}

// callQwen sends a single-turn chat request to a local Ollama server and
// returns the model's text response. No timeout is set deliberately —
// CPU-only local inference can be slow, and cutting it off early would
// just turn a working setup into a flaky one.
func callQwen(ctx context.Context, system, userMessage string) (string, error) {
	body := map[string]interface{}{
		"model": qwenModel(),
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": userMessage},
		},
		"stream": false,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ollamaURL(), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling Ollama at %s (is the ollama container/service running and the model pulled?): %w", ollamaURL(), err)
	}
	defer resp.Body.Close()

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode Ollama response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("Ollama error: %s (has the model been pulled? try: docker compose run --rm pull-model)", result.Error)
	}
	if result.Message.Content == "" {
		return "", fmt.Errorf("empty response from Ollama")
	}
	return result.Message.Content, nil
}

// aiJSONReply is the strict shape we ask the model to respond in, so
// "products" can be grounded in the real catalog rather than trusting
// the model to state correct IDs/prices/stock counts in free text.
// Smaller local models follow formatting instructions less reliably than
// hosted frontier models, so parseAIReply below falls back gracefully
// rather than erroring the whole request when this isn't followed exactly.
type aiJSONReply struct {
	Answer     string   `json:"answer"`
	ProductIDs []string `json:"productIds"`
}

const jsonReplyInstruction = `Respond with ONLY a single JSON object, no other text, no markdown code fences, matching exactly this shape:
{"answer": "<your natural-language answer>", "productIds": ["<id1>", "<id2>"]}
"productIds" should list the id field of any specific catalog products your answer refers to, in relevance order. Use an empty array if none apply.`

func parseAIReply(raw string, catalog []Product) (*AssistantReply, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	// Smaller models sometimes add a sentence before/after the JSON
	// despite instructions — try to isolate the outermost {...} block
	// before giving up and falling back to plain text.
	if start := strings.Index(cleaned, "{"); start != -1 {
		if end := strings.LastIndex(cleaned, "}"); end != -1 && end > start {
			cleaned = cleaned[start : end+1]
		}
	}

	var parsed aiJSONReply
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return &AssistantReply{Answer: raw, Products: []Product{}}, nil
	}

	byID := make(map[string]Product, len(catalog))
	for _, p := range catalog {
		byID[p.ID] = p
	}

	products := make([]Product, 0, len(parsed.ProductIDs))
	for _, id := range parsed.ProductIDs {
		if p, ok := byID[id]; ok {
			products = append(products, p)
		}
	}
	return &AssistantReply{Answer: parsed.Answer, Products: products}, nil
}

func catalogAsJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// --- Customer side ---

// 4. Natural-language product search.
func SearchProducts(ctx context.Context, query string) (*AssistantReply, error) {
	catalog, err := listAllProducts(ctx)
	if err != nil {
		return nil, err
	}

	maxPrice := extractMaxPrice(query)
	filtered := filterByMaxPrice(catalog, maxPrice)

	system := fmt.Sprintf(`You are a hotel's product search assistant. Full current catalog (JSON):
%s

A guest is searching in natural language. Find the best-matching in-stock items (stock > 0) and briefly explain why each fits. If nothing matches well, say so honestly rather than forcing a match.%s

%s`, catalogAsJSON(filtered), priceConstraintNote(maxPrice), jsonReplyInstruction)

	raw, err := callQwen(ctx, system, query)
	if err != nil {
		return nil, err
	}
	reply, err := parseAIReply(raw, filtered)
	if err != nil {
		return nil, err
	}
	reply.Products = filterByMaxPrice(reply.Products, maxPrice) // defense in depth
	return reply, nil
}

// 5. Recommendations, optionally nudged by free-text context.
func RecommendProducts(ctx context.Context, freeTextContext *string) (*AssistantReply, error) {
	catalog, err := listAllProducts(ctx)
	if err != nil {
		return nil, err
	}

	userMessage := "Recommend something for me."
	if freeTextContext != nil && strings.TrimSpace(*freeTextContext) != "" {
		userMessage = *freeTextContext
	}

	maxPrice := extractMaxPrice(userMessage)
	filtered := filterByMaxPrice(catalog, maxPrice)

	system := fmt.Sprintf(`You are a hotel's product recommendation assistant. Full current catalog (JSON):
%s

Recommend 2-4 in-stock items (stock > 0). If the guest gave context (dietary needs, occasion, mood), tailor to it; otherwise suggest well-rounded popular picks across categories.%s

%s`, catalogAsJSON(filtered), priceConstraintNote(maxPrice), jsonReplyInstruction)

	raw, err := callQwen(ctx, system, userMessage)
	if err != nil {
		return nil, err
	}
	reply, err := parseAIReply(raw, filtered)
	if err != nil {
		return nil, err
	}
	reply.Products = filterByMaxPrice(reply.Products, maxPrice) // defense in depth
	return reply, nil
}

// 8. Substitutes for a specific (often out-of-stock) product.
func SubstituteProducts(ctx context.Context, productID string) (*AssistantReply, error) {
	catalog, err := listAllProducts(ctx)
	if err != nil {
		return nil, err
	}
	target, err := getProduct(ctx, productID)
	if err != nil {
		return nil, err
	}

	system := fmt.Sprintf(`You are a hotel's product substitution assistant. Full current catalog (JSON):
%s

A guest wants an alternative to this item (it may be out of stock, or they may just prefer another option):
%s

Suggest 1-3 in-stock (stock > 0) substitutes from the same or a similar category, briefly explaining why each is a reasonable swap.

%s`, catalogAsJSON(catalog), catalogAsJSON(target), jsonReplyInstruction)

	raw, err := callQwen(ctx, system, fmt.Sprintf("Suggest substitutes for %q.", target.Name))
	if err != nil {
		return nil, err
	}
	return parseAIReply(raw, catalog)
}

// --- Employee side ---

// 7. Stock-aware Q&A: what's low, what to reorder, etc.
func StockInsights(ctx context.Context, question string) (*AssistantReply, error) {
	catalog, err := listAllProducts(ctx)
	if err != nil {
		return nil, err
	}
	system := fmt.Sprintf(`You are a stock-management assistant for hotel staff. Full current catalog with live stock counts (JSON):
%s

Answer using these real numbers — flag what's low or out of stock, suggest reorder priorities, or answer whatever was specifically asked. Be concise and concrete; cite actual item names and stock counts.

%s`, catalogAsJSON(catalog), jsonReplyInstruction)

	raw, err := callQwen(ctx, system, question)
	if err != nil {
		return nil, err
	}
	return parseAIReply(raw, catalog)
}

// 9. General staff assistant — broader scope, still grounded in the
// catalog/stock as available context.
func StaffAssistant(ctx context.Context, question string) (*AssistantReply, error) {
	catalog, err := listAllProducts(ctx)
	if err != nil {
		return nil, err
	}
	system := fmt.Sprintf(`You are a general-purpose assistant for hotel staff. You have the current product catalog and stock levels as JSON below, useful context if relevant, but you should help with any staff question (procedures, guest requests, general advice) even when unrelated to the catalog:
%s

%s`, catalogAsJSON(catalog), jsonReplyInstruction)

	raw, err := callQwen(ctx, system, question)
	if err != nil {
		return nil, err
	}
	return parseAIReply(raw, catalog)
}
