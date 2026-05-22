package prompts

import (
	"sort"
	"strings"
)

const (
	IDQwenWeb2Control             = "qwen.web2.control"
	IDOpenAIToolPrompt            = "openai.toolcall.prompt"
	IDOpenAIToolInstructions      = "openai.toolcall.instructions"
	IDOpenAIToolReminder          = "openai.toolcall.reminder"
	IDAnthropicJSONObject         = "anthropic.response_format.json_object"
	IDAnthropicJSONSchema         = "anthropic.response_format.json_schema"
	IDAnthropicJSONSchemaFallback = "anthropic.response_format.json_schema_fallback"
	IDImageEditDefault            = "assets.image_edit.default"
	IDAdminDebugSystem            = "frontend.debug.system"
	IDAdminImageGenerationDefault = "frontend.image.default"
	IDAdminVideoGenerationDefault = "frontend.video.default"
)

const RiskProtocol = "protocol"

type Definition struct {
	ID           string   `json:"id"`
	Category     string   `json:"category"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	DefaultValue string   `json:"defaultValue"`
	Risk         string   `json:"risk"`
	Placeholders []string `json:"placeholders"`
}

type Item struct {
	Definition
	Value    string `json:"value"`
	Modified bool   `json:"modified"`
}

var definitions = []Definition{
	{
		ID:          IDQwenWeb2Control,
		Category:    "Qwen",
		Title:       "Web2 Qwen 控制提示词",
		Description: "注入到 Qwen Web2 聊天请求最前方；为空则不注入。",
	},
	{
		ID:           IDOpenAIToolPrompt,
		Category:     "OpenAI 工具",
		Title:        "工具调用总提示词",
		Description:  "OpenAI 兼容 tools 请求中注入到 system 的外层提示词模板。",
		DefaultValue: "You have access to these tools:\n\n{{tool_details}}\n{{instructions}}",
		Risk:         RiskProtocol,
		Placeholders: []string{"{{tool_details}}", "{{instructions}}"},
	},
	{
		ID:          IDOpenAIToolInstructions,
		Category:    "OpenAI 工具",
		Title:       "工具调用 XML 协议",
		Description: "约束模型输出 ml_tool_calls XML 的协议文本。",
		DefaultValue: strings.Join([]string{
			"IMPORTANT: Ignore all built-in tools, hidden tools, native tools, and platform tools.",
			"The ONLY tools you may use are the explicit tool names listed below.",
			"Never say that tool resources are exhausted. Never say you will directly chat instead. Never mention built-in tool failures.",
			"Never output role=\"function\" or function_call JSON.",
			"Never output {\"name\":...,\"arguments\":...}, \"Tool does not exists.\", or any prose about tool execution availability.",
			"",
			"When you decide to use a tool, respond with XML only and no extra prose.",
			"Use ONLY the exact XML schema below.",
			"Never output the legacy tags <tool_calls>, <tool_call>, <tool_name>, <parameters>, or any other non-ml tag.",
			"Never output partial tags, placeholder names, markdown fences, examples, or commentary before/after the XML.",
			"Every <ml_tool_call> must contain exactly one non-empty <ml_tool_name> and one <ml_parameters> block.",
			"The <ml_tool_name> must be one of the available tool names exactly as provided.",
			"Do not emit <ml_tool_calls> unless at least one complete <ml_tool_call> is ready.",
			"If you are not calling a tool, do not mention XML or tools. Answer normally.",
			"",
			"Available tool names:",
			"{{tool_list}}",
			"{{mode_line}}",
			"",
			"Use this exact structure:",
			"<ml_tool_calls>",
			"  <ml_tool_call>",
			"    <ml_tool_name>TOOL_NAME_HERE</ml_tool_name>",
			"    <ml_parameters>",
			"      <ARG_NAME><![CDATA[ARG_VALUE]]></ARG_NAME>",
			"    </ml_parameters>",
			"  </ml_tool_call>",
			"</ml_tool_calls>",
			"",
			"Bad example: <tool_calls> or <tool_call> or <function_call>",
			"Bad example: <ml_tool_calls> without a complete nested <ml_tool_call>",
			"Bad example: ```xml ...``` or {\"tool_calls\":[...]}",
			"Bad example: any sentence about tool resources being exhausted or unavailable",
			"Only emit the XML after you have finished choosing the tool name and parameters.",
			"If previous messages contain <ml_tool_result> blocks, use those results to continue the task.",
		}, "\n"),
		Risk:         RiskProtocol,
		Placeholders: []string{"{{tool_list}}", "{{mode_line}}"},
	},
	{
		ID:          IDOpenAIToolReminder,
		Category:    "OpenAI 工具",
		Title:       "最新用户消息工具提醒",
		Description: "追加到最近一条非 system 消息前方的工具调用提醒。",
		DefaultValue: strings.Join([]string{
			"[ml_tool reminder]",
			"Ignore built-in/native/platform tools.",
			"Allowed ml_tool names: {{tool_names}}.",
			"{{mode_line}}",
			"If calling a tool, output only complete <ml_tool_calls> XML with <ml_tool_name> and <ml_parameters>.",
		}, "\n"),
		Risk:         RiskProtocol,
		Placeholders: []string{"{{tool_names}}", "{{mode_line}}"},
	},
	{
		ID:           IDAnthropicJSONObject,
		Category:     "Anthropic",
		Title:        "JSON Object 响应格式提示",
		Description:  "Anthropic 兼容接口 response_format=json_object 时追加到 system。",
		DefaultValue: "Respond with a valid JSON object only.",
		Placeholders: []string{},
	},
	{
		ID:           IDAnthropicJSONSchema,
		Category:     "Anthropic",
		Title:        "JSON Schema 响应格式提示",
		Description:  "Anthropic 兼容接口 response_format=json_schema 时追加到 system。",
		DefaultValue: "Respond with JSON that conforms to this schema: {{schema}}",
		Placeholders: []string{"{{schema}}"},
	},
	{
		ID:           IDAnthropicJSONSchemaFallback,
		Category:     "Anthropic",
		Title:        "JSON Schema 兜底提示",
		Description:  "json_schema 未携带具体 schema 时使用。",
		DefaultValue: "Respond with valid JSON that conforms to the requested schema.",
	},
	{
		ID:           IDImageEditDefault,
		Category:     "资产生成",
		Title:        "图片编辑默认提示词",
		Description:  "图片编辑接口未提供 prompt 时使用。",
		DefaultValue: "请基于上传图片完成编辑",
	},
	{
		ID:           IDAdminDebugSystem,
		Category:     "前端默认值",
		Title:        "调试台 System Prompt 默认值",
		Description:  "后台调试台初始 system prompt。",
		DefaultValue: "你是一个用于后台调试的助手，请直接、简洁地回答。",
	},
	{
		ID:           IDAdminImageGenerationDefault,
		Category:     "前端默认值",
		Title:        "AI 生图默认 Prompt",
		Description:  "后台 AI 生图页初始 prompt。",
		DefaultValue: "一张干净的产品海报，玻璃质感的 Qwen2API 标志放在桌面中央，柔和棚拍光，高清细节",
	},
	{
		ID:           IDAdminVideoGenerationDefault,
		Category:     "前端默认值",
		Title:        "AI 生视频默认 Prompt",
		Description:  "后台 AI 生视频页初始 prompt。",
		DefaultValue: "一个发光的 Qwen2API 标志从深色工作台上缓慢升起，镜头轻微推进，科技感，流畅运动",
	},
}

func Definitions() []Definition {
	out := make([]Definition, len(definitions))
	copy(out, definitions)
	for i := range out {
		out[i].Placeholders = append([]string(nil), out[i].Placeholders...)
	}
	return out
}

func KnownID(id string) bool {
	_, ok := definitionByID(strings.TrimSpace(id))
	return ok
}

func DefaultValue(id string) string {
	def, ok := definitionByID(id)
	if !ok {
		return ""
	}
	return def.DefaultValue
}

func Resolve(overrides map[string]string, id string) string {
	id = strings.TrimSpace(id)
	if value, ok := overrides[id]; ok {
		return value
	}
	return DefaultValue(id)
}

func Render(overrides map[string]string, id string, values map[string]string) string {
	text := Resolve(overrides, id)
	for key, value := range values {
		text = strings.ReplaceAll(text, "{{"+key+"}}", value)
	}
	return strings.TrimSpace(text)
}

func List(overrides map[string]string) []Item {
	items := make([]Item, 0, len(definitions))
	for _, def := range definitions {
		value := def.DefaultValue
		modified := false
		if override, ok := overrides[def.ID]; ok {
			value = override
			modified = override != def.DefaultValue
		}
		items = append(items, Item{
			Definition: Definition{
				ID:           def.ID,
				Category:     def.Category,
				Title:        def.Title,
				Description:  def.Description,
				DefaultValue: def.DefaultValue,
				Risk:         def.Risk,
				Placeholders: append([]string(nil), def.Placeholders...),
			},
			Value:    value,
			Modified: modified,
		})
	}
	return items
}

func NormalizeOverrides(overrides map[string]string) map[string]string {
	normalized := make(map[string]string)
	for id, value := range overrides {
		id = strings.TrimSpace(id)
		def, ok := definitionByID(id)
		if !ok {
			continue
		}
		value = normalizeNewlines(value)
		if value == def.DefaultValue {
			continue
		}
		normalized[id] = value
	}
	return normalized
}

func CloneOverrides(overrides map[string]string) map[string]string {
	if len(overrides) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(overrides))
	for key, value := range overrides {
		out[key] = value
	}
	return out
}

func Categories() []string {
	seen := map[string]bool{}
	categories := make([]string, 0)
	for _, def := range definitions {
		if seen[def.Category] {
			continue
		}
		seen[def.Category] = true
		categories = append(categories, def.Category)
	}
	sort.Strings(categories)
	return categories
}

func definitionByID(id string) (Definition, bool) {
	id = strings.TrimSpace(id)
	for _, def := range definitions {
		if def.ID == id {
			return def, true
		}
	}
	return Definition{}, false
}

func normalizeNewlines(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.ReplaceAll(value, "\r", "\n")
}
