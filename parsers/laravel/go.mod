module github.com/piligrim/marvindocs/parsers/laravel

go 1.25

require (
	github.com/gomarkdown/markdown v0.0.0-20240930133441-72d49d9543d8
	github.com/piligrim/marvindocs/parsers/devdocsfmt v0.0.0
)

replace github.com/piligrim/marvindocs/parsers/devdocsfmt => ../devdocsfmt
