package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestGoTypeToTS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		expr         string
		wantNullable bool
		wantType     string
	}{
		{name: "string", expr: "string", wantType: "string"},
		{name: "number", expr: "int64", wantType: "number"},
		{name: "pointer", expr: "*time.Time", wantNullable: true, wantType: "string"},
		{name: "selector", expr: "uuid.UUID", wantType: "string"},
		{name: "byte slice", expr: "[]byte", wantType: "string"},
		{name: "slice", expr: "[]string", wantType: "string[]"},
		{name: "nested slice", expr: "[][]int", wantType: "number[][]"},
		{name: "slice of pointers", expr: "[]*time.Time", wantType: "(string | null)[]"},
		{name: "string map", expr: "map[string]int", wantType: "{ [key: string]: number }"},
		{name: "number map", expr: "map[int]string", wantType: "{ [key: number]: string }"},
		{name: "fallback key map", expr: "map[bool]string", wantType: "{ [key: string]: string }"},
		{name: "interface", expr: "interface{}", wantType: "any"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			expr := parseExpr(t, tt.expr)
			gotNullable, gotType := goTypeToTS(expr)
			if gotNullable != tt.wantNullable {
				t.Fatalf("nullable mismatch: got %v, want %v", gotNullable, tt.wantNullable)
			}
			if gotType != tt.wantType {
				t.Fatalf("type mismatch: got %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestParseJSONTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		tag           *ast.BasicLit
		wantName      string
		wantOmitempty bool
		wantSkip      bool
	}{
		{name: "nil tag", tag: nil},
		{name: "json name", tag: &ast.BasicLit{Value: "`json:\"created_at\"`"}, wantName: "created_at"},
		{name: "omitempty", tag: &ast.BasicLit{Value: "`json:\"expires_at,omitempty\"`"}, wantName: "expires_at", wantOmitempty: true},
		{name: "skip", tag: &ast.BasicLit{Value: "`json:\"-\"`"}, wantSkip: true},
		{name: "no json", tag: &ast.BasicLit{Value: "`db:\"id\"`"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotName, gotOmitempty, gotSkip := parseJSONTag(tt.tag)
			if gotName != tt.wantName || gotOmitempty != tt.wantOmitempty || gotSkip != tt.wantSkip {
				t.Fatalf("got (%q,%v,%v), want (%q,%v,%v)", gotName, gotOmitempty, gotSkip, tt.wantName, tt.wantOmitempty, tt.wantSkip)
			}
		})
	}
}

func TestConvertStruct(t *testing.T) {
	t.Parallel()

	st := parseStructType(t, `package p
import "time"
type ApiKey struct {
	ID string `+"`json:\"id\"`"+`
	CreatedAt time.Time `+"`json:\"created_at\"`"+`
	ExpiresAt *time.Time `+"`json:\"expires_at,omitempty\"`"+`
	Hash []byte `+"`json:\"-\"`"+`
	Name string
	private string
}
`)

	got, err := convertStruct("ApiKey", st)
	if err != nil {
		t.Fatalf("convertStruct returned error: %v", err)
	}

	if got.name != "ApiKey" {
		t.Fatalf("name mismatch: got %q", got.name)
	}

	if len(got.fields) != 4 {
		t.Fatalf("field count mismatch: got %d, want %d", len(got.fields), 4)
	}

	type wantField struct {
		name     string
		tsType   string
		nullable bool
		optional bool
	}
	want := []wantField{
		{name: "id", tsType: "string"},
		{name: "created_at", tsType: "string"},
		{name: "expires_at", tsType: "string", nullable: true, optional: true},
		{name: "Name", tsType: "string"},
	}

	for i, w := range want {
		f := got.fields[i]
		if f.name != w.name || f.tsType != w.tsType || f.nullable != w.nullable || f.optional != w.optional {
			t.Fatalf("field[%d] got (%q,%q,%v,%v), want (%q,%q,%v,%v)", i, f.name, f.tsType, f.nullable, f.optional, w.name, w.tsType, w.nullable, w.optional)
		}
	}
}

func parseExpr(t *testing.T, src string) ast.Expr {
	t.Helper()
	expr, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("parse expr %q: %v", src, err)
	}
	return expr
}

func parseStructType(t *testing.T, src string) *ast.StructType {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "in.go", src, 0)
	if err != nil {
		t.Fatalf("parse file: %v", err)
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if ok {
				return st
			}
		}
	}
	t.Fatal("no struct type found")
	return nil
}
