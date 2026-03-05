package nai

import (
	"fmt"
	"strconv"
	"strings"
)

func resolveDimensions(shapeMap map[string]string, shape string, defaultShape string) (int, int, error) {
	if shape == "" {
		shape = defaultShape
	}
	dims := shapeMap[shape]
	if dims == "" {
		return 0, 0, fmt.Errorf("未配置 shape=%s 的尺寸", shape)
	}
	parts := strings.Split(dims, "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("非法尺寸格式: %s", dims)
	}
	w, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("非法宽度: %w", err)
	}
	h, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("非法高度: %w", err)
	}
	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("宽高必须 > 0")
	}
	return w, h, nil
}
