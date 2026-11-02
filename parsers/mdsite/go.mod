module github.com/piligrim/marvindocs/parsers/mdsite

go 1.25

require (
	github.com/gomarkdown/markdown v0.0.0-20260923180740-94fc73f6b1a3
	github.com/piligrim/marvindocs/parsers/devdocsfmt v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/piligrim/marvindocs/parsers/devdocsfmt => ../devdocsfmt
