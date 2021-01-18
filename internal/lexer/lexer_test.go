package lexer_test

import (
	"log"
	"regexp"
	"testing"

	"github.com/wader/bump/internal/lexer"
)

func TestOr(t *testing.T) {

	// var linkTitle string
	// var linkURL string
	// tokens := []lexer.Token{
	// 	{Name: "title", Dest: &linkTitle, Fn: lexer.Or(lexer.Quoted(`"`), lexer.Re(regexp.MustCompile(`\w`)))},
	// 	{Fn: lexer.Re(regexp.MustCompile(`\s`))},
	// 	{Name: "URL", Dest: &linkURL, Fn: lexer.Rest(1)},
	// }

	// if _, err := lexer.Tokenize(`tjo hej`, tokens); err != nil {
	// 	t.Error(err)
	// }

	// log.Printf("linkTitle: %#+v\n", linkTitle)
	// log.Printf("linkURL: %#+v\n", linkURL)

	// tokens = []lexer.Token{
	// 	{Name: "title", Dest: &linkTitle, Fn: lexer.Or(lexer.Quoted(`"`), lexer.Re(regexp.MustCompile(`\w`)))},
	// 	{Fn: lexer.Re(regexp.MustCompile(`\s`))},
	// 	{Name: "URL", Dest: &linkURL, Fn: lexer.Rest(1)},
	// }
	// if _, err := lexer.Tokenize(`"hej asdas" tjo`, tokens); err != nil {
	// 	t.Error(err)
	// }

	var currentReStr string
	var pipelineStr string
	p, err := lexer.Scan(`/name: (1)/ static:2`,
		lexer.Concat(
			lexer.T{Name: "re", Dest: &currentReStr, Fn: lexer.Quoted(`/`)},
			lexer.T{Fn: lexer.Re(regexp.MustCompile(`\s`))},
			lexer.T{Name: "pipeline", Dest: &pipelineStr, Fn: lexer.Rest(1)},
		),
	)

	log.Printf("p: %#+v\n", p)
	log.Printf("err: %#+v\n", err)
	log.Printf("currentReStr: %#+v\n", currentReStr)
	log.Printf("pipelineStr: %#+v\n", pipelineStr)

}
