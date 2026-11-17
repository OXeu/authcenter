package configcenter

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// ErrInvalidDotenv identifies malformed dotenv input without exposing its contents.
var ErrInvalidDotenv = errors.New("configcenter: invalid dotenv syntax")

// DotenvSource reads files using godotenv without changing the process environment.
// Files are ordered from highest to lowest priority. Nil reads an optional .env;
// an empty slice disables the source. Explicitly named files must exist.
type DotenvSource struct {
	Files []string
}

// Read reloads dotenv files and returns entries matching prefix.
func (s DotenvSource) Read(ctx context.Context, prefix string) ([]Item, error) {
	files := s.Files
	if files == nil {
		files = []string{".env"}
	}
	sources := make(Sources, 0, len(files))
	var failures []error
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("configcenter: read dotenv: %w", err)
		}
		values, err := godotenv.Read(file)
		if err != nil {
			if s.Files == nil && errors.Is(err, os.ErrNotExist) {
				continue
			}
			var pathErr *os.PathError
			if errors.As(err, &pathErr) {
				failures = append(failures, fmt.Errorf("configcenter: read dotenv %q: %w", file, err))
			} else {
				failures = append(failures, fmt.Errorf("configcenter: parse dotenv %q: %w", file, ErrInvalidDotenv))
			}
			continue
		}
		sources = append(sources, MapSource(values))
	}
	items, err := sources.Read(ctx, prefix)
	return items, errors.Join(append(failures, err)...)
}
