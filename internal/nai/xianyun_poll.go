package nai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"grimoire/internal/types"
)

func (c *XianyunClient) Poll(ctx context.Context, jobID string) (types.JobResult, error) {
	cfg := c.cfg.Snapshot()
	url := strings.TrimRight(cfg.NAI.BaseURL, "/") + "/get_result/" + jobID

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return types.JobResult{}, fmt.Errorf("创建轮询请求失败: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+cfg.NAI.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return types.JobResult{}, fmt.Errorf("轮询请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.JobResult{}, fmt.Errorf("读取轮询响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return types.JobResult{}, fmt.Errorf("轮询失败: status=%d body=%s", resp.StatusCode, truncate(string(body), 400))
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return types.JobResult{}, fmt.Errorf("解析轮询响应失败: %w", err)
	}

	result := types.JobResult{}
	result.Status = toString(raw["status"])
	result.ImageBase64 = toString(raw["image_base64"])
	result.Error = toString(raw["error"])

	if q, ok := raw["queue_position"]; ok {
		result.QueuePosition = toInt(q)
	}

	if result.Status == "" {
		return types.JobResult{}, fmt.Errorf("轮询响应缺少 status")
	}

	return result, nil
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}
