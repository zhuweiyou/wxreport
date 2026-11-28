package theme

import (
	_ "embed"
	"encoding/base64"
	"html/template"
	"strings"
)

// 微信群昵称里常见 ᰔᩚ（Lepcha + Tai Tham）装饰字符，Windows/macOS 默认都没有对应字体，
// 不内嵌就会截成方框。字体只用于截图 HTML，所有主题共享这一份。
//
//go:embed fonts/NotoSansLepcha-Regular.ttf
var lepchaFont []byte

//go:embed fonts/NotoSansTaiTham-Regular.ttf
var taiThamFont []byte

var sharedFontCSS = func() template.CSS {
	var builder strings.Builder
	builder.WriteString(`@font-face{font-family:"Shot Lepcha";src:url(data:font/ttf;base64,`)
	builder.WriteString(base64.StdEncoding.EncodeToString(lepchaFont))
	builder.WriteString(`) format("truetype");}`)
	builder.WriteString(`@font-face{font-family:"Shot Tai Tham";src:url(data:font/ttf;base64,`)
	builder.WriteString(base64.StdEncoding.EncodeToString(taiThamFont))
	builder.WriteString(`) format("truetype");}`)
	return template.CSS(builder.String())
}()

// SharedFontCSS 返回所有主题共用的字体 @font-face 声明。
func SharedFontCSS() template.CSS {
	return sharedFontCSS
}
