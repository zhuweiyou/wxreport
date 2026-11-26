// Package compress 用本地颜色量化压缩报告长图，不依赖网络或 native 组件。
package compress

import (
	"bytes"
	"fmt"
	"image/png"

	"github.com/soniakeys/quant/median"
)

// PNG 把 PNG 量化到 256 色并用 BestCompression 重新编码。
// 调用方应比较输出与输入大小，无收益时保留原图。
func PNG(data []byte) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("解码 PNG: %w", err)
	}

	paletted := median.Quantizer(256).Paletted(img)

	var buf bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&buf, paletted); err != nil {
		return nil, fmt.Errorf("编码 PNG: %w", err)
	}
	return buf.Bytes(), nil
}
