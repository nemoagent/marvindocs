package main

import (
	"strings"
	"testing"
)

func TestFrigateClean(t *testing.T) {
	in := strings.Join([]string{
		`<NavPath path="Settings > Cameras" />`,
		`<FaqItem id="x" question={<>Why?</>}>`,
		"<Tabs\ngroupId=\"g\"\nvalues={[\n{ label: \"UI\", value: \"ui\" },\n]}\n\n> <TabItem value=\"ui\">",
		"<details>\n<summary>More</summary>\n```yaml\na: 1\n```\n</details>",
	}, "\n")
	got := cleanMarkdown(home2Sites["frigate"], in, nil, 0)
	for _, want := range []string{"**Settings > Cameras**", "### Why?", "**UI**", "**More**"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{"<Tabs", "<TabItem", "<FaqItem", "<details", "groupId"} {
		if strings.Contains(got, bad) {
			t.Errorf("unexpected %q in:\n%s", bad, got)
		}
	}
}

func TestESPHomeClean(t *testing.T) {
	in := strings.Join([]string{
		`- <APIRef text="wifi.h" path="wifi/wifi.h" />`,
		"<Figure\n  src={img}\n  caption=\"A sensor.\"\n/>",
		"<ImgTable items={[\n  [\"YAML\", \"/guides/yaml/\", \"a.svg\"],\n]} />",
		"<EnumValues values={[\n  { value: \"A\", default: true },\n  { value: \"B\", description: \"Second\" },\n]} />",
		"- ```yaml\n  a: 1\n  ```",
		"Use <TOPIC> here",
	}, "\n")
	got := cleanMarkdown(home2Sites["esphome"], in, nil, 0)
	for _, want := range []string{"`wifi.h`", "*A sensor.*", "- [YAML](/guides/yaml/)", "- `A` (default)", "- `B`: Second", "- Example:\n  ```yaml", "<TOPIC>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{"<APIRef", "<Figure", "<ImgTable", "<EnumValues"} {
		if strings.Contains(got, bad) {
			t.Errorf("unexpected %q in:\n%s", bad, got)
		}
	}
}

func TestHACSClean(t *testing.T) {
	files := map[string]string{"source/docs/faq/highlights.md": "---\ntitle: x\n---\nline1\n- item"}
	in := strings.Join([]string{
		`Go to <!-- hacs:my updates title="**{{coreui('panel.config')}}** > **{{coreui('ui.panel.config.updates.caption')}}**" -->.`,
		`Menu :material-dots-vertical: and [a](/docs/use/data.md/#b){ .md-button }`,
		`--8<-- "source/docs/faq/highlights.md:5"`,
	}, "\n")
	got := cleanMarkdown(home2Sites["hacs"], in, files, 0)
	for _, want := range []string{"**Settings** > **Updates**", "[a](/docs/use/data.md#b)", "- item"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{"hacs:my", "coreui", ":material", "md-button"} {
		if strings.Contains(got, bad) {
			t.Errorf("unexpected %q in:\n%s", bad, got)
		}
	}
}
