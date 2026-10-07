package olehdtv

import (
	"context"
	"fmt"
	"path/filepath"
)

// SourceAdapter loads authorized OLEHDTV / MacCMS catalog data.
type SourceAdapter interface {
	Name() string
	ListCategories(ctx context.Context) ([]SourceCategory, error)
	ListVideos(ctx context.Context) ([]SourceVideo, error)
}

// NewAdapter picks an adapter from OLEHDTV_SOURCE_MODE.
// Modes: fixture | export_file | maccms_db | html_dump
func NewAdapter(mode, dsn, exportPath, fixturePath, htmlDumpRoot, baseURL string) (SourceAdapter, error) {
	switch mode {
	case "", "fixture":
		return NewFixtureAdapter(fixturePath)
	case "export_file", "export":
		return NewExportFileAdapter(exportPath)
	case "maccms_db", "db":
		return NewMacCMSDBAdapter(dsn)
	case "html_dump", "maccms_html", "dump":
		if htmlDumpRoot == "" {
			htmlDumpRoot = filepath.Join("testdata", "maccms_html")
		}
		return NewHTMLDumpAdapter(htmlDumpRoot, baseURL)
	default:
		return nil, fmt.Errorf("unknown OLEHDTV_SOURCE_MODE %q", mode)
	}
}
