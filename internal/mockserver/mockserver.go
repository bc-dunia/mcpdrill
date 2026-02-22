package mockserver

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/mcp"
	"github.com/bc-dunia/mcpdrill/internal/types"
)

// Config configures the mock server.
type Config struct {
	Addr     string
	behavior BehaviorProfile
}

func (c *Config) SetBehavior(b *BehaviorProfile) {
	if b == nil {
		return
	}
	c.behavior = *b
}

func DefaultConfig() *Config {
	return &Config{
		Addr: "127.0.0.1:0",
		behavior: BehaviorProfile{
			StreamingChunkCount:   5,
			StreamingChunkDelayMs: 50,
		},
	}
}

// BehaviorProfile controls streaming behavior.
type BehaviorProfile struct {
	StreamingChunkCount   int
	StreamingChunkDelayMs int
}

// Server is the mock server interface.
type Server interface {
	Start() error
	Stop(ctx context.Context)
	Addr() string
	MCPURL() string
}

// New creates a new mock server.
func New(config *Config) Server {
	if config == nil {
		config = DefaultConfig()
	}
	return &mockServer{
		cfg:          config,
		behavior:     config.behavior,
		rateLimiter:  newRateLimiter(5, time.Second),
		backpressure: make(chan struct{}, 5),
	}
}

// StartTestServer starts a server with defaults and returns cleanup.
func StartTestServer() (server Server, cleanup func()) {
	cfg := DefaultConfig()
	srv := New(cfg)
	if err := srv.Start(); err != nil {
		return srv, func() {}
	}
	cleanup = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Stop(ctx)
	}
	return srv, cleanup
}

type mockServer struct {
	cfg          *Config
	behavior     BehaviorProfile
	httpServer   *http.Server
	listener     net.Listener
	addr         string
	stateCounter atomic.Int64
	degradeCount atomic.Int64

	mu            sync.Mutex
	circuitFails  int
	circuitOpenTo time.Time
	rateLimiter   *tokenBucket
	backpressure  chan struct{}
}

func (s *mockServer) Start() error {
	if s.cfg == nil {
		s.cfg = DefaultConfig()
	}

	listenAddr := normalizeAddr(s.cfg.Addr)
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	s.listener = ln
	s.addr = ln.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.handleMCP)

	s.httpServer = &http.Server{
		Handler: mux,
	}

	go func() {
		_ = s.httpServer.Serve(ln)
	}()

	return nil
}

func (s *mockServer) Stop(ctx context.Context) {
	if s.httpServer == nil {
		return
	}
	_ = s.httpServer.Shutdown(ctx)
}

func (s *mockServer) Addr() string {
	return s.addr
}

func (s *mockServer) MCPURL() string {
	if s.addr == "" {
		return ""
	}
	return "http://" + s.addr + "/mcp"
}

func (s *mockServer) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := ioReadAll(r)
	if err != nil {
		writeJSONRPCError(w, nil, -32700, "failed to read body")
		return
	}

	var req types.JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONRPCError(w, nil, -32700, "invalid json")
		return
	}

	if req.JSONRPC != "2.0" {
		writeJSONRPCError(w, req.ID, -32600, "invalid jsonrpc version")
		return
	}

	switch req.Method {
	case "initialize":
		var initParams types.InitializeParams
		if req.Params != nil {
			json.Unmarshal(req.Params, &initParams)
		}
		version := initParams.ProtocolVersion
		if version == "" || !mcp.IsSupported(version) {
			version = mcp.DefaultProtocolVersion
		}
		result := types.InitializeResult{
			ProtocolVersion: version,
			Capabilities:    map[string]interface{}{},
			ServerInfo:      types.ServerInfo{Name: "mockserver", Version: "1.0.0"},
		}
		writeJSONRPCResult(w, req.ID, result)
		return
	case "ping":
		writeJSONRPCResult(w, req.ID, map[string]interface{}{"ok": true})
		return
	case "tools/list":
		result := types.ToolsListResult{Tools: buildToolsList()}
		writeJSONRPCResult(w, req.ID, result)
		return
	case "tools/call":
		s.handleToolsCall(w, r, req)
		return
	case "resources/list":
		result := buildResourcesList()
		writeJSONRPCResult(w, req.ID, result)
		return
	case "resources/read":
		s.handleResourcesRead(w, req)
		return
	case "prompts/list":
		result := buildPromptsList()
		writeJSONRPCResult(w, req.ID, result)
		return
	case "prompts/get":
		s.handlePromptsGet(w, req)
		return
	default:
		writeJSONRPCError(w, req.ID, -32601, "method not found")
		return
	}
}

func (s *mockServer) handleToolsCall(w http.ResponseWriter, r *http.Request, req types.JSONRPCRequest) {
	var params types.ToolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		result := toolErrorResult("invalid params")
		writeJSONRPCResult(w, req.ID, result)
		return
	}

	if params.Name == "streaming_tool" && acceptsSSE(r) {
		s.handleStreamingTool(w, r, req.ID, params.Arguments)
		return
	}

	result, ok := s.executeTool(r.Context(), params.Name, params.Arguments)
	if !ok {
		result = toolErrorResult("unknown tool")
	}

	writeJSONRPCResult(w, req.ID, result)
}

func (s *mockServer) handleStreamingTool(w http.ResponseWriter, r *http.Request, id interface{}, args map[string]interface{}) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	chunks, delay := s.streamingParams(args)
	if chunks <= 0 {
		chunks = 1
	}

	ctx := r.Context()
	for i := 0; i < chunks; i++ {
		progress := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "progress",
			"id":      id,
			"params": map[string]interface{}{
				"chunk": i + 1,
				"total": chunks,
			},
		}
		if !writeSSE(w, progress) {
			return
		}
		flusher.Flush()

		if i < chunks-1 && delay > 0 {
			if !sleepWithContext(ctx, time.Duration(delay)*time.Millisecond) {
				return
			}
		}
	}

	finalResult := types.ToolsCallResult{
		Content: []types.ToolContent{{Type: "text", Text: "stream complete"}},
	}
	finalResp := types.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
	}
	finalPayload, _ := json.Marshal(finalResult)
	finalResp.Result = finalPayload

	if !writeSSE(w, finalResp) {
		return
	}
	flusher.Flush()
}

func (s *mockServer) streamingParams(args map[string]interface{}) (int, int) {
	chunks := s.behavior.StreamingChunkCount
	delay := s.behavior.StreamingChunkDelayMs

	if v, ok := args["chunks"]; ok {
		if n, ok := toInt(v); ok && n > 0 {
			chunks = n
		}
	}
	if v, ok := args["delay_ms"]; ok {
		if n, ok := toInt(v); ok && n >= 0 {
			delay = n
		}
	}
	return chunks, delay
}

func (s *mockServer) executeTool(ctx context.Context, name string, args map[string]interface{}) (types.ToolsCallResult, bool) {
	switch name {
	case "fast_echo":
		msg, ok := getStringArg(args, "message")
		if !ok {
			return toolErrorResult("missing message"), true
		}
		return textResult("echo: " + msg), true
	case "slow_echo":
		msg, ok := getStringArg(args, "message")
		if !ok {
			return toolErrorResult("missing message"), true
		}
		_ = sleepWithContext(ctx, 200*time.Millisecond)
		return textResult("echo: " + msg), true
	case "error_tool":
		return toolErrorResult("error_tool invoked"), true
	case "timeout_tool":
		<-ctx.Done()
		return types.ToolsCallResult{}, true
	case "streaming_tool":
		return textResult("streaming complete"), true
	case "json_transform":
		return jsonTransform(args), true
	case "text_processor":
		return textProcessor(args), true
	case "list_operations":
		return listOperations(args), true
	case "validate_email":
		return validateEmail(args), true
	case "calculate":
		return calculateExpression(args), true
	case "hash_generator":
		return hashGenerator(args), true
	case "weather_api":
		return weatherAPI(args), true
	case "geocode":
		return geocode(args), true
	case "currency_convert":
		return currencyConvert(args), true
	case "read_file":
		return readFile(args), true
	case "write_file":
		return writeFile(args), true
	case "list_directory":
		return listDirectory(args), true
	case "large_payload":
		return largePayload(args), true
	case "random_latency":
		return randomLatency(ctx, args), true
	case "conditional_error":
		return conditionalError(args), true
	case "degrading_performance":
		return s.degradingPerformance(ctx), true
	case "flaky_connection":
		return flakyConnection(), true
	case "rate_limited":
		return s.rateLimited(), true
	case "circuit_breaker":
		return s.circuitBreaker(args), true
	case "backpressure":
		return s.backpressureTool(ctx), true
	case "stateful_counter":
		return s.statefulCounter(), true
	case "realistic_latency":
		return realisticLatency(ctx), true
	default:
		return types.ToolsCallResult{}, false
	}
}

func buildToolsList() []types.Tool {
	schema := json.RawMessage(`{"type":"object"}`)
	names := []string{
		"fast_echo", "slow_echo", "error_tool", "timeout_tool", "streaming_tool",
		"json_transform", "text_processor", "list_operations",
		"validate_email", "calculate", "hash_generator",
		"weather_api", "geocode", "currency_convert",
		"read_file", "write_file", "list_directory",
		"large_payload", "random_latency", "conditional_error",
		"degrading_performance", "flaky_connection", "rate_limited",
		"circuit_breaker", "backpressure", "stateful_counter", "realistic_latency",
	}

	tools := make([]types.Tool, 0, len(names))
	for _, name := range names {
		tools = append(tools, types.Tool{
			Name:        name,
			Description: "mock tool",
			InputSchema: schema,
		})
	}
	return tools
}

// buildResourcesList returns a list of mock resources.
func buildResourcesList() types.ResourcesListResult {
	return types.ResourcesListResult{
		Resources: []types.Resource{
			{
				URI:         "file:///docs/readme.md",
				Name:        "README",
				Description: "Project documentation",
				MimeType:    "text/markdown",
			},
			{
				URI:         "file:///config/settings.json",
				Name:        "Settings",
				Description: "Application configuration",
				MimeType:    "application/json",
			},
			{
				URI:         "file:///data/sample.csv",
				Name:        "Sample Data",
				Description: "Sample CSV data file",
				MimeType:    "text/csv",
			},
		},
	}
}

// handleResourcesRead handles the resources/read method.
func (s *mockServer) handleResourcesRead(w http.ResponseWriter, req types.JSONRPCRequest) {
	var params types.ResourcesReadParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, -32602, "invalid params")
		return
	}

	if params.URI == "" {
		writeJSONRPCError(w, req.ID, -32602, "missing uri parameter")
		return
	}

	// Return mock content based on URI
	var content types.ResourceContent
	switch params.URI {
	case "file:///docs/readme.md":
		content = types.ResourceContent{
			URI:      params.URI,
			MimeType: "text/markdown",
			Text:     "# Mock Project\n\nThis is a mock README file for testing purposes.\n\n## Features\n\n- Feature 1\n- Feature 2\n- Feature 3\n",
		}
	case "file:///config/settings.json":
		content = types.ResourceContent{
			URI:      params.URI,
			MimeType: "application/json",
			Text:     `{"debug": true, "timeout": 30, "maxRetries": 3}`,
		}
	case "file:///data/sample.csv":
		content = types.ResourceContent{
			URI:      params.URI,
			MimeType: "text/csv",
			Text:     "id,name,value\n1,alpha,100\n2,beta,200\n3,gamma,300\n",
		}
	default:
		// Return generic content for unknown URIs
		content = types.ResourceContent{
			URI:      params.URI,
			MimeType: "text/plain",
			Text:     fmt.Sprintf("Mock content for resource: %s", params.URI),
		}
	}

	result := types.ResourcesReadResult{
		Contents: []types.ResourceContent{content},
	}
	writeJSONRPCResult(w, req.ID, result)
}

// buildPromptsList returns a list of mock prompts.
func buildPromptsList() types.PromptsListResult {
	return types.PromptsListResult{
		Prompts: []types.Prompt{
			{
				Name:        "summarize",
				Description: "Summarize the given text",
				Arguments: []types.PromptArgument{
					{Name: "text", Description: "Text to summarize", Required: true},
					{Name: "max_length", Description: "Maximum summary length", Required: false},
				},
			},
			{
				Name:        "translate",
				Description: "Translate text to another language",
				Arguments: []types.PromptArgument{
					{Name: "text", Description: "Text to translate", Required: true},
					{Name: "target_language", Description: "Target language code", Required: true},
				},
			},
			{
				Name:        "code_review",
				Description: "Review code for issues and improvements",
				Arguments: []types.PromptArgument{
					{Name: "code", Description: "Code to review", Required: true},
					{Name: "language", Description: "Programming language", Required: false},
				},
			},
		},
	}
}

// handlePromptsGet handles the prompts/get method.
func (s *mockServer) handlePromptsGet(w http.ResponseWriter, req types.JSONRPCRequest) {
	var params types.PromptsGetParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, -32602, "invalid params")
		return
	}

	if params.Name == "" {
		writeJSONRPCError(w, req.ID, -32602, "missing name parameter")
		return
	}

	var result types.PromptsGetResult
	switch params.Name {
	case "summarize":
		text, _ := params.Arguments["text"].(string)
		if text == "" {
			text = "[no text provided]"
		}
		result = types.PromptsGetResult{
			Description: "Summarize the given text",
			Messages: []types.PromptMessage{
				{
					Role: "user",
					Content: types.PromptContent{
						Type: "text",
						Text: fmt.Sprintf("Please summarize the following text:\n\n%s", text),
					},
				},
			},
		}
	case "translate":
		text, _ := params.Arguments["text"].(string)
		targetLang, _ := params.Arguments["target_language"].(string)
		if text == "" {
			text = "[no text provided]"
		}
		if targetLang == "" {
			targetLang = "en"
		}
		result = types.PromptsGetResult{
			Description: "Translate text to another language",
			Messages: []types.PromptMessage{
				{
					Role: "user",
					Content: types.PromptContent{
						Type: "text",
						Text: fmt.Sprintf("Translate the following text to %s:\n\n%s", targetLang, text),
					},
				},
			},
		}
	case "code_review":
		code, _ := params.Arguments["code"].(string)
		language, _ := params.Arguments["language"].(string)
		if code == "" {
			code = "[no code provided]"
		}
		langHint := ""
		if language != "" {
			langHint = fmt.Sprintf(" (written in %s)", language)
		}
		result = types.PromptsGetResult{
			Description: "Review code for issues and improvements",
			Messages: []types.PromptMessage{
				{
					Role: "user",
					Content: types.PromptContent{
						Type: "text",
						Text: fmt.Sprintf("Please review the following code%s for potential issues, bugs, and improvements:\n\n```\n%s\n```", langHint, code),
					},
				},
			},
		}
	default:
		writeJSONRPCError(w, req.ID, -32602, fmt.Sprintf("unknown prompt: %s", params.Name))
		return
	}

	writeJSONRPCResult(w, req.ID, result)
}

func writeJSONRPCResult(w http.ResponseWriter, id interface{}, result interface{}) {
	resp := types.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
	}
	payload, _ := json.Marshal(result)
	resp.Result = payload

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeJSONRPCError(w http.ResponseWriter, id interface{}, code int, message string) {
	resp := types.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &types.JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeSSE(w http.ResponseWriter, payload interface{}) bool {
	data, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return false
	}
	if _, err := w.Write(data); err != nil {
		return false
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return false
	}
	return true
}

func acceptsSSE(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

func normalizeAddr(addr string) string {
	if addr == "" {
		return "127.0.0.1:0"
	}
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" {
		return "127.0.0.1:" + port
	}
	return addr
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func toolErrorResult(msg string) types.ToolsCallResult {
	return types.ToolsCallResult{
		Content: []types.ToolContent{{Type: "text", Text: msg}},
		IsError: true,
	}
}

func textResult(text string) types.ToolsCallResult {
	return types.ToolsCallResult{
		Content: []types.ToolContent{{Type: "text", Text: text}},
	}
}

func jsonTransform(args map[string]interface{}) types.ToolsCallResult {
	op, ok := getStringArg(args, "operation")
	if !ok {
		return toolErrorResult("missing operation")
	}
	data, ok := args["data"].(map[string]interface{})
	if !ok {
		return toolErrorResult("invalid data")
	}

	var result map[string]interface{}
	switch op {
	case "uppercase_keys":
		result = transformKeys(data, strings.ToUpper)
	case "lowercase_values":
		result = transformValues(data, strings.ToLower)
	case "filter":
		key, ok := getStringArg(args, "filter_key")
		if !ok {
			return toolErrorResult("missing filter_key")
		}
		if value, ok := data[key]; ok {
			result = map[string]interface{}{key: value}
		} else {
			result = map[string]interface{}{}
		}
	default:
		return toolErrorResult("unknown operation")
	}

	payload, _ := json.Marshal(result)
	return textResult(string(payload))
}

func transformKeys(data map[string]interface{}, fn func(string) string) map[string]interface{} {
	out := make(map[string]interface{}, len(data))
	for k, v := range data {
		nk := fn(k)
		if child, ok := v.(map[string]interface{}); ok {
			out[nk] = transformKeys(child, fn)
		} else {
			out[nk] = v
		}
	}
	return out
}

func transformValues(data map[string]interface{}, fn func(string) string) map[string]interface{} {
	out := make(map[string]interface{}, len(data))
	for k, v := range data {
		switch tv := v.(type) {
		case string:
			out[k] = fn(tv)
		case map[string]interface{}:
			out[k] = transformValues(tv, fn)
		default:
			out[k] = v
		}
	}
	return out
}

func textProcessor(args map[string]interface{}) types.ToolsCallResult {
	text, ok := getStringArg(args, "text")
	if !ok {
		return toolErrorResult("missing text")
	}
	op, ok := getStringArg(args, "operation")
	if !ok {
		return toolErrorResult("missing operation")
	}
	switch op {
	case "uppercase":
		return textResult(strings.ToUpper(text))
	case "lowercase":
		return textResult(strings.ToLower(text))
	case "reverse":
		return textResult(reverseString(text))
	default:
		return toolErrorResult("unknown operation")
	}
}

func reverseString(input string) string {
	runes := []rune(input)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func listOperations(args map[string]interface{}) types.ToolsCallResult {
	list, ok := getFloatListArg(args, "list")
	if !ok || len(list) == 0 {
		return toolErrorResult("invalid list")
	}
	op, ok := getStringArg(args, "operation")
	if !ok {
		return toolErrorResult("missing operation")
	}

	switch op {
	case "sum":
		return textResult(formatFloat(sum(list)))
	case "avg":
		return textResult(formatFloat(sum(list) / float64(len(list))))
	case "sort":
		sorted := append([]float64(nil), list...)
		sort.Float64s(sorted)
		return textResult(formatFloatSlice(sorted))
	case "filter":
		fv, ok := getFloatArg(args, "filter_value")
		if !ok {
			return toolErrorResult("missing filter_value")
		}
		var filtered []float64
		for _, v := range list {
			if v > fv {
				filtered = append(filtered, v)
			}
		}
		return textResult(formatFloatSlice(filtered))
	case "max":
		maxVal := list[0]
		for _, v := range list[1:] {
			if v > maxVal {
				maxVal = v
			}
		}
		return textResult(formatFloat(maxVal))
	default:
		return toolErrorResult("unknown operation")
	}
}

func sum(list []float64) float64 {
	var total float64
	for _, v := range list {
		total += v
	}
	return total
}

func validateEmail(args map[string]interface{}) types.ToolsCallResult {
	email, ok := getStringArg(args, "email")
	if !ok {
		return toolErrorResult("missing email")
	}
	valid := strings.Contains(email, "@") && strings.Contains(email, ".")
	payload := fmt.Sprintf(`{"email":"%s","valid":%t}`, email, valid)
	return textResult(payload)
}

func calculateExpression(args map[string]interface{}) types.ToolsCallResult {
	expr, ok := getStringArg(args, "expression")
	if !ok {
		return toolErrorResult("missing expression")
	}
	value, err := evalExpression(expr)
	if err != nil {
		return toolErrorResult("invalid expression")
	}
	return textResult(formatFloat(value))
}

func hashGenerator(args map[string]interface{}) types.ToolsCallResult {
	data, ok := getStringArg(args, "data")
	if !ok {
		return toolErrorResult("missing data")
	}
	algo, ok := getStringArg(args, "algorithm")
	if !ok {
		return toolErrorResult("missing algorithm")
	}
	switch strings.ToLower(algo) {
	case "md5":
		sum := md5.Sum([]byte(data))
		return textResult(hex.EncodeToString(sum[:]))
	case "sha256":
		sum := sha256.Sum256([]byte(data))
		return textResult(hex.EncodeToString(sum[:]))
	default:
		return toolErrorResult("unknown algorithm")
	}
}

func weatherAPI(args map[string]interface{}) types.ToolsCallResult {
	city, ok := getStringArg(args, "city")
	if !ok {
		return toolErrorResult("missing city")
	}
	units := "celsius"
	if u, ok := getStringArg(args, "units"); ok {
		units = u
	}
	payload := fmt.Sprintf(`{"city":"%s","units":"%s","temp":20}`, city, units)
	return textResult(payload)
}

func geocode(args map[string]interface{}) types.ToolsCallResult {
	address, ok := getStringArg(args, "address")
	if !ok {
		return toolErrorResult("missing address")
	}
	payload := fmt.Sprintf(`{"address":"%s","lat":51.5074,"lon":-0.1278}`, address)
	return textResult(payload)
}

func currencyConvert(args map[string]interface{}) types.ToolsCallResult {
	amount, ok := getFloatArg(args, "amount")
	if !ok {
		return toolErrorResult("missing amount")
	}
	from, ok := getStringArg(args, "from")
	if !ok {
		return toolErrorResult("missing from")
	}
	to, ok := getStringArg(args, "to")
	if !ok {
		return toolErrorResult("missing to")
	}

	rate, ok := lookupRate(strings.ToUpper(from), strings.ToUpper(to))
	if !ok {
		return toolErrorResult("unsupported currency")
	}
	converted := amount * rate
	payload := fmt.Sprintf(`{"amount":%s,"from":"%s","to":"%s","converted":%s}`,
		formatFloat(amount), strings.ToUpper(from), strings.ToUpper(to), formatFloat(converted))
	return textResult(payload)
}

func lookupRate(from, to string) (float64, bool) {
	if from == to {
		return 1.0, true
	}
	rates := map[string]float64{
		"USD:EUR": 0.9,
		"EUR:USD": 1.1,
		"USD:GBP": 0.8,
		"GBP:USD": 1.25,
	}
	rate, ok := rates[from+":"+to]
	return rate, ok
}

func readFile(args map[string]interface{}) types.ToolsCallResult {
	path, ok := getStringArg(args, "path")
	if !ok {
		return toolErrorResult("missing path")
	}
	payload := fmt.Sprintf(`{"path":"%s","content":"mock content"}`, path)
	return textResult(payload)
}

func writeFile(args map[string]interface{}) types.ToolsCallResult {
	_, ok := getStringArg(args, "path")
	if !ok {
		return toolErrorResult("missing path")
	}
	_, ok = getStringArg(args, "content")
	if !ok {
		return toolErrorResult("missing content")
	}
	return textResult("success")
}

func listDirectory(args map[string]interface{}) types.ToolsCallResult {
	path, ok := getStringArg(args, "path")
	if !ok {
		return toolErrorResult("missing path")
	}
	payload := fmt.Sprintf(`{"path":"%s","entries":["file1.txt","file2.txt"]}`, path)
	return textResult(payload)
}

func largePayload(args map[string]interface{}) types.ToolsCallResult {
	sizeKB, ok := getFloatArg(args, "size_kb")
	if !ok || sizeKB <= 0 {
		return toolErrorResult("invalid size_kb")
	}
	size := int(math.Round(sizeKB * 1024))
	if size < 1 {
		size = 1
	}
	return textResult(strings.Repeat("a", size))
}

func randomLatency(ctx context.Context, args map[string]interface{}) types.ToolsCallResult {
	minMs, ok := getFloatArg(args, "min_ms")
	if !ok {
		return toolErrorResult("missing min_ms")
	}
	maxMs, ok := getFloatArg(args, "max_ms")
	if !ok {
		return toolErrorResult("missing max_ms")
	}
	if maxMs < minMs {
		return toolErrorResult("invalid range")
	}
	delay := minMs + rand.Float64()*(maxMs-minMs)
	_ = sleepWithContext(ctx, time.Duration(delay)*time.Millisecond)
	return textResult(fmt.Sprintf("slept %dms", int(delay)))
}

func conditionalError(args map[string]interface{}) types.ToolsCallResult {
	prob, ok := getFloatArg(args, "error_probability")
	if !ok {
		return toolErrorResult("missing error_probability")
	}
	if rand.Float64() < prob {
		return toolErrorResult("conditional error")
	}
	return textResult("ok")
}

func (s *mockServer) degradingPerformance(ctx context.Context) types.ToolsCallResult {
	count := s.degradeCount.Add(1)
	_ = sleepWithContext(ctx, time.Duration(count*20)*time.Millisecond)
	return textResult(fmt.Sprintf("degraded step %d", count))
}

func flakyConnection() types.ToolsCallResult {
	if rand.Float64() < 0.3 {
		return toolErrorResult("flaky connection")
	}
	return textResult("ok")
}

func (s *mockServer) rateLimited() types.ToolsCallResult {
	if !s.rateLimiter.Allow() {
		return toolErrorResult("rate limited")
	}
	return textResult("ok")
}

func (s *mockServer) circuitBreaker(args map[string]interface{}) types.ToolsCallResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	if time.Now().Before(s.circuitOpenTo) {
		return toolErrorResult("circuit open")
	}

	if fail, ok := getBoolArg(args, "force_error"); ok && fail {
		s.circuitFails++
	} else {
		s.circuitFails = 0
	}

	if s.circuitFails >= 3 {
		s.circuitOpenTo = time.Now().Add(2 * time.Second)
		s.circuitFails = 0
		return toolErrorResult("circuit open")
	}

	return textResult("ok")
}

func (s *mockServer) backpressureTool(ctx context.Context) types.ToolsCallResult {
	select {
	case s.backpressure <- struct{}{}:
		defer func() { <-s.backpressure }()
	case <-ctx.Done():
		return toolErrorResult("cancelled")
	case <-time.After(100 * time.Millisecond):
		return toolErrorResult("backpressure")
	}
	_ = sleepWithContext(ctx, 50*time.Millisecond)
	return textResult("ok")
}

func (s *mockServer) statefulCounter() types.ToolsCallResult {
	value := s.stateCounter.Add(1)
	return textResult(strconv.FormatInt(value, 10))
}

func realisticLatency(ctx context.Context) types.ToolsCallResult {
	delay := 50 + rand.Intn(100)
	_ = sleepWithContext(ctx, time.Duration(delay)*time.Millisecond)
	return textResult(fmt.Sprintf("slept %dms", delay))
}

func getStringArg(args map[string]interface{}, key string) (string, bool) {
	if args == nil {
		return "", false
	}
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func getFloatArg(args map[string]interface{}, key string) (float64, bool) {
	if args == nil {
		return 0, false
	}
	v, ok := args[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func getBoolArg(args map[string]interface{}, key string) (bool, bool) {
	if args == nil {
		return false, false
	}
	v, ok := args[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func getFloatListArg(args map[string]interface{}, key string) ([]float64, bool) {
	if args == nil {
		return nil, false
	}
	raw, ok := args[key]
	if !ok {
		return nil, false
	}
	list, ok := raw.([]interface{})
	if !ok {
		if f64s, ok := raw.([]float64); ok {
			return f64s, true
		}
		return nil, false
	}
	out := make([]float64, 0, len(list))
	for _, item := range list {
		if v, ok := item.(float64); ok {
			out = append(out, v)
		} else {
			return nil, false
		}
	}
	return out, true
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func formatFloatSlice(values []float64) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, formatFloat(v))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func toInt(value interface{}) (int, bool) {
	switch n := value.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

type tokenBucket struct {
	capacity int
	tokens   int
	lastFill time.Time
	mu       sync.Mutex
	window   time.Duration
}

func newRateLimiter(capacity int, window time.Duration) *tokenBucket {
	return &tokenBucket{
		capacity: capacity,
		tokens:   capacity,
		lastFill: time.Now(),
		window:   window,
	}
}

func (t *tokenBucket) Allow() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	if now.Sub(t.lastFill) >= t.window {
		t.tokens = t.capacity
		t.lastFill = now
	}

	if t.tokens <= 0 {
		return false
	}
	t.tokens--
	return true
}

func ioReadAll(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

// Expression evaluator

func evalExpression(expr string) (float64, error) {
	tokens, err := tokenize(expr)
	if err != nil {
		return 0, err
	}
	output, err := shuntingYard(tokens)
	if err != nil {
		return 0, err
	}
	return evalRPN(output)
}

func tokenize(expr string) ([]string, error) {
	var tokens []string
	i := 0
	for i < len(expr) {
		ch := expr[i]
		if ch == ' ' || ch == '\t' || ch == '\n' {
			i++
			continue
		}
		if isDigit(ch) || ch == '.' {
			start := i
			i++
			for i < len(expr) && (isDigit(expr[i]) || expr[i] == '.') {
				i++
			}
			tokens = append(tokens, expr[start:i])
			continue
		}
		if strings.ContainsRune("+-*/()", rune(ch)) {
			tokens = append(tokens, string(ch))
			i++
			continue
		}
		return nil, fmt.Errorf("invalid character")
	}
	return tokens, nil
}

func shuntingYard(tokens []string) ([]string, error) {
	var output []string
	var ops []string
	precedence := map[string]int{"+": 1, "-": 1, "*": 2, "/": 2}

	for _, tok := range tokens {
		if isNumber(tok) {
			output = append(output, tok)
			continue
		}
		switch tok {
		case "+", "-", "*", "/":
			for len(ops) > 0 {
				top := ops[len(ops)-1]
				if top == "(" {
					break
				}
				if precedence[top] >= precedence[tok] {
					output = append(output, top)
					ops = ops[:len(ops)-1]
					continue
				}
				break
			}
			ops = append(ops, tok)
		case "(":
			ops = append(ops, tok)
		case ")":
			found := false
			for len(ops) > 0 {
				top := ops[len(ops)-1]
				ops = ops[:len(ops)-1]
				if top == "(" {
					found = true
					break
				}
				output = append(output, top)
			}
			if !found {
				return nil, fmt.Errorf("mismatched parens")
			}
		default:
			return nil, fmt.Errorf("invalid token")
		}
	}

	for len(ops) > 0 {
		top := ops[len(ops)-1]
		ops = ops[:len(ops)-1]
		if top == "(" || top == ")" {
			return nil, fmt.Errorf("mismatched parens")
		}
		output = append(output, top)
	}

	return output, nil
}

func evalRPN(tokens []string) (float64, error) {
	var stack []float64
	for _, tok := range tokens {
		if isNumber(tok) {
			v, err := strconv.ParseFloat(tok, 64)
			if err != nil {
				return 0, err
			}
			stack = append(stack, v)
			continue
		}
		if len(stack) < 2 {
			return 0, fmt.Errorf("invalid expression")
		}
		b := stack[len(stack)-1]
		a := stack[len(stack)-2]
		stack = stack[:len(stack)-2]
		switch tok {
		case "+":
			stack = append(stack, a+b)
		case "-":
			stack = append(stack, a-b)
		case "*":
			stack = append(stack, a*b)
		case "/":
			if b == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			stack = append(stack, a/b)
		default:
			return 0, fmt.Errorf("invalid operator")
		}
	}
	if len(stack) != 1 {
		return 0, fmt.Errorf("invalid expression")
	}
	return stack[0], nil
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isNumber(token string) bool {
	if token == "" {
		return false
	}
	_, err := strconv.ParseFloat(token, 64)
	return err == nil
}
