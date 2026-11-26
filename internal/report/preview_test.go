package report

import (
	"html/template"
	"os"
	"path/filepath"
	"testing"
)

// 仅用于 UI 迭代：REPORT_PREVIEW=1 go test ./internal/report -run TestPreviewHTML
// 用 mock 数据把 static/report-v2.* 渲染到 data/report-preview.html。
func TestPreviewHTML(t *testing.T) {
	if os.Getenv("REPORT_PREVIEW") == "" {
		t.Skip("设置 REPORT_PREVIEW=1 生成预览")
	}

	tmplSource, err := os.ReadFile("static/report-v2.html.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	css, err := os.ReadFile("static/report-v2.css")
	if err != nil {
		t.Fatal(err)
	}
	tmpl := template.Must(template.New("preview").Parse(string(tmplSource)))

	type participant struct {
		Name  string
		Count int
	}
	type resource struct {
		Index int
		Title string
		URL   string
	}
	type segment struct {
		Kind  string
		Text  string
		Index int
		URL   string
	}
	type topic struct {
		Rank         int
		RankClass    string
		Title        string
		Commentary   string
		Segments     []segment
		Participants []participant
		Resources    []resource
	}

	resources := []resource{
		{1, "腾讯视频会员租号产业链调查", "https://mp.weixin.qq.com/s?__biz=Mzg5MzEyNzEwNQ==&mid=2247878502&idx=1&sn=ae01e8e7a3fbe04db5f4714dce2d6c75"},
		{2, "闲鱼「超前点播」关键词搜索结果", "https://www.goofish.com/search?q=%E8%B6%85%E5%89%8D%E7%82%B9%E6%92%AD"},
		{3, "杭州 E 类人才补贴政策原文", "https://hrss.hangzhou.gov.cn/art/2024/3/15/art_1229416358_4245678.html"},
		{4, "外卖券包退款规则（客服口径）", "https://mp.weixin.qq.com/s?__biz=MzA5NzQxNTk2MA==&mid=2651234567&idx=2&sn=9f8e7d6c5b4a3210"},
	}

	topics := []topic{
		{
			Rank:       1,
			RankClass:  "top1",
			Title:      "腾讯视频超前点映与闲鱼租号生意",
			Commentary: "从超前点映到会员租号，群友把「信息差」三个字研究成了一门灰色生意。",
			Segments: []segment{
				{Kind: "text", Text: "摩达ᴹᴼᴰᴬ 透露「之前展开给我扫码登录的号，到现在还能刷新 Cookie」，"},
				{Kind: "person", Text: "Not Be丶HHH"},
				{Kind: "text", Text: " 追问有多少个 vip 号，他回「0」，随后承认是批量跑任务、还要抓接口，目标是「搞超前点播券」。"},
				{Kind: "resource", Index: 1, URL: "https://example.com/a"},
				{Kind: "text", Text: " 群里已有人「两天搞了几百个」。"},
			},
			Participants: []participant{
				{"摩达ᴹᴼᴰᴬ", 18}, {"Not Be丶HHH", 12}, {"Lie", 7}, {"zhu哥哥", 6},
				{"娶你需要注意什么", 5}, {"一只聪明娜娜ᰔᩚ", 4}, {"timeless", 3}, {"A777", 2},
				{"拉屎时人类也短暂拥有过尾巴", 2}, {"梳子🎏", 1},
			},
			Resources: resources[:2],
		},
		{
			Rank:       2,
			RankClass:  "top2",
			Title:      "从 gay 酒局到洛阳铲相亲局",
			Commentary: "「有男朋友又不是有老公，还有机会」——一句复读，基本概括了本群的全部婚恋观。",
			Segments: []segment{
				{Kind: "person", Text: "某乜嘢。"},
				{Kind: "text", Text: " 深夜被拉去一个酒局，全程直播社恐：「特么的等会进去四五个 gay 就我一个直男」；"},
				{Kind: "person", Text: "Lie"},
				{Kind: "text", Text: " 要给 "},
				{Kind: "person", Text: "拉屎时人类也短暂拥有过尾巴"},
				{Kind: "text", Text: " 介绍洛阳男生，众人刷「又一个南通」。"},
			},
			Participants: []participant{
				{"某乜嘢。", 14}, {"Lie", 11}, {"A777", 8}, {"timeless", 6},
				{"拉屎时人类也短暂拥有过尾巴", 5}, {"Steve.", 4}, {"飞白", 3},
				{"你的网友小半", 3}, {"Burgess", 2}, {"hockergd", 2}, {"woainisxx", 1},
			},
		},
		{
			Rank:       3,
			RankClass:  "top3",
			Title:      "血橙、草莓妹妹与骑手自取",
			Commentary: "一箱血橙引出妹妹照片和全群复读，最后没人关心血橙，只关心「老吃家」。",
			Segments: []segment{
				{Kind: "person", Text: "72521（西西红柿版"},
				{Kind: "text", Text: " 晒门口水果自取，众人追问妹妹微信和血橙味道；"},
				{Kind: "person", Text: "zhu哥哥"},
				{Kind: "text", Text: " 说感受不出区别。全群复读「老吃家」。"},
				{Kind: "resource", Index: 2, URL: "https://example.com/b"},
			},
			Participants: []participant{
				{"72521（西西红柿版", 16}, {"zhu哥哥", 9}, {"Muddy", 5}, {"7", 4},
				{"拉屎时人类也短暂拥有过尾巴", 4}, {"海", 3}, {"梳子🎏", 2}, {"lemon亮", 1},
			},
			Resources: resources[1:2],
		},
		{
			Rank:       4,
			RankClass:  "",
			Title:      "软考、杭州 E 类人才与亚运村",
			Commentary: "从软考难度聊到杭州补贴，最后发现证有用但首付没有。",
			Segments: []segment{
				{Kind: "person", Text: "Facksxx"},
				{Kind: "text", Text: " 问 "},
				{Kind: "person", Text: "zhu哥哥"},
				{Kind: "text", Text: " 软考高级，"},
				{Kind: "person", Text: "飞白"},
				{Kind: "text", Text: " 嫌不是专家掉价；众人算账：杭州 E 类人才补贴大、买房便宜几十万。"},
				{Kind: "resource", Index: 3, URL: "https://example.com/c"},
			},
			Participants: []participant{
				{"Facksxx", 8}, {"zhu哥哥", 7}, {"飞白", 5}, {"Burgess", 4},
				{"Lie", 4}, {"拉屎时人类也短暂拥有过尾巴", 3}, {"Not Be丶HHH", 2},
			},
			Resources: resources[2:3],
		},
		{
			Rank:       5,
			RankClass:  "",
			Title:      "一块钱瑞幸：外卖神券教程",
			Commentary: "券包涨到 3.6 后，群友的主线从「怎么领」变成「怎么退、怎么换设备、怎么卡明天刷新」。",
			Segments: []segment{
				{Kind: "person", Text: "Not Be丶HHH"},
				{Kind: "text", Text: " 继续答疑：券包每个号不一样、第二天会刷新；"},
				{Kind: "person", Text: "娶你需要注意什么"},
				{Kind: "text", Text: " 发现券包涨到 3.6，哀嚎「我的0元奶茶啊」。"},
				{Kind: "resource", Index: 4, URL: "https://example.com/d"},
			},
			Participants: []participant{{"Not Be丶HHH", 10}, {"阿毛", 6}, {"timeless", 4}},
			Resources:    resources[3:4],
		},
	}

	data := map[string]any{
		"Title":            "「他们连一箱螃蟹都没送3群」24 小时话题 TOP10",
		"PageTitle":        "「他们连一箱螃蟹都没送3群」24 小时话题 TOP10",
		"ChatName":         "他们连一箱螃蟹都没送3群",
		"WindowStart":      "11-25 20:00",
		"WindowEnd":        "11-26 20:00",
		"MessageTotal":     2531,
		"ParticipantTotal": 62,
		"TopicTotal":       len(topics),
		"Topics":           topics,
		"Resources":        resources,
		"CSS":              template.CSS(css),
		"FontCSS":          template.CSS(rareFontCSS),
	}

	out := filepath.Join("..", "..", "data", "report-preview.html")
	file, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if err := tmpl.Execute(file, data); err != nil {
		t.Fatal(err)
	}
	t.Logf("已生成 %s", out)
}
