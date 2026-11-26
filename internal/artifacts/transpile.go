package artifacts

import (
	"errors"
	"fmt"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// Compile turns model-written source into the bytes served by artifact://.
// JSX is transpiled in-process by esbuild; HTML is kept as a document and the
// handler injects the import map and bridge bootstrap when serving it.
func Compile(source string, kind SourceKind) (compiled, mime string, err error) {
	if err := validateSource(source); err != nil {
		return "", "", err
	}
	switch kind {
	case SourceHTML:
		return source, "text/html; charset=utf-8", nil
	case SourceJSX:
		result := api.Transform(source, api.TransformOptions{
			Loader: api.LoaderJSX,
			JSX:    api.JSXTransform,
			Format: api.FormatESModule,
			Target: api.ESNext,
		})
		if len(result.Errors) > 0 {
			messages := make([]string, 0, len(result.Errors))
			for _, msg := range result.Errors {
				if msg.Location != nil {
					messages = append(messages, fmt.Sprintf("line %d: %s", msg.Location.Line, msg.Text))
					continue
				}
				messages = append(messages, msg.Text)
			}
			return "", "", fmt.Errorf("esbuild JSX transform failed: %s", strings.Join(messages, "; "))
		}
		return string(result.Code), "text/javascript; charset=utf-8", nil
	default:
		return "", "", fmt.Errorf("unsupported artifact source kind %q", kind)
	}
}

// TranspileJSX is the narrow, testable seam for esbuild transformation.
func TranspileJSX(source string) (string, error) {
	code, _, err := Compile(source, SourceJSX)
	return code, err
}

// ErrTranspile is used by callers that need to detect a failed transform.
var ErrTranspile = errors.New("artifact transpile failed")
