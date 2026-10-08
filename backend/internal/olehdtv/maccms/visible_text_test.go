package maccms_test

import (
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

func TestVisibleText_ignoresScriptStyleNoscriptTemplate(t *testing.T) {
	in := `
<span>
  <script>var vod_id = 82861; var dadww = $.cookie(vod_id.toString());</script>
  <style>.x{color:red}</style>
  <noscript>enable js</noscript>
  <template><span>hidden</span></template>
  功夫女足
</span>`
	got := maccms.VisibleText(in)
	if got != "功夫女足" {
		t.Fatalf("got %q", got)
	}
}

func TestVisibleText_scriptAfterAndNested(t *testing.T) {
	if got := maccms.VisibleText(`兰香如故<script>var x=1</script>`); got != "兰香如故" {
		t.Fatalf("after: %q", got)
	}
	if got := maccms.VisibleText(`<b><script>var x=1</script>披荆斩棘</b>`); got != "披荆斩棘" {
		t.Fatalf("nested: %q", got)
	}
}

func TestParseDetail_titleIgnoresInlineScriptInH1(t *testing.T) {
	cookieJS := `var vod_id = 82861;var dadww = $.cookie(vod_id.toString());if(dadww!=null){$(".scookie").css("color","rgb(252,173,3)");}`
	html := `
<html><body>
<div class="myui-content__detail">
  <h1 class="title">
    <script>` + cookieJS + `</script>
    功夫女足
  </h1>
  <p>导演：张导演<script>var d=1</script></p>
  <p>主演：演员甲</p>
  <p>地区：中国大陆</p>
  <p>年份：2023</p>
  <div class="sketch content">剧情简介<script>var y=2</script>正文</div>
</div>
<div class="playlist">
  <a href="/index.php/vod/play/id/82861/sid/1/nid/1.html">正片</a>
  <a href="/index.php/vod/play/id/82861/sid/1/nid/2.html">第2集</a>
</div>
</body></html>`
	meta := maccms.ParseDetailPage(html, "https://maccms.local/index.php/vod/detail/id/82861.html")
	if meta.Title != "功夫女足" {
		t.Fatalf("title=%q", meta.Title)
	}
	if strings.Contains(meta.Title, "vod_id") || strings.Contains(meta.Title, "cookie") {
		t.Fatalf("JS leaked into title: %q", meta.Title)
	}
	if meta.Director != "张导演" {
		t.Fatalf("director=%q", meta.Director)
	}
	if strings.Contains(meta.Description, "var y") {
		t.Fatalf("JS leaked into description: %q", meta.Description)
	}
	if len(meta.Episodes) != 2 {
		t.Fatalf("episodes=%d", len(meta.Episodes))
	}
}

func TestParseDetail_titleScriptAfterText(t *testing.T) {
	html := `
<html><body>
<div class="myui-content__detail">
  <h1 class="title">兰香如故<script>var vod_id = 84001;var dadww = $.cookie(vod_id.toString());</script></h1>
</div>
<div class="playlist"><a href="/index.php/vod/play/id/84001/sid/1/nid/1.html">第1集</a></div>
</body></html>`
	meta := maccms.ParseDetailPage(html, "https://maccms.local/index.php/vod/detail/id/84001.html")
	if meta.Title != "兰香如故" {
		t.Fatalf("title=%q", meta.Title)
	}
}

func TestParseDetail_plainChineseTitleUnchanged(t *testing.T) {
	html := `
<html><body>
<div class="myui-content__detail"><h1 class="title">披荆斩棘</h1></div>
<div class="playlist"><a href="/index.php/vod/play/id/83449/sid/1/nid/1.html">正片</a></div>
</body></html>`
	meta := maccms.ParseDetailPage(html, "https://maccms.local/index.php/vod/detail/id/83449.html")
	if meta.Title != "披荆斩棘" {
		t.Fatalf("title=%q", meta.Title)
	}
}

func TestStripNonVisibleElements_keepsLabels(t *testing.T) {
	in := `<p>导演：甲<script>var x=1</script></p><style>.a{}</style><p>年份：2024</p>`
	out := maccms.StripNonVisibleElements(in)
	if strings.Contains(out, "var x") || strings.Contains(strings.ToLower(out), "<script") {
		t.Fatalf("script remained: %s", out)
	}
	if !strings.Contains(out, "导演：甲") || !strings.Contains(out, "2024") {
		t.Fatalf("labels lost: %s", out)
	}
}
