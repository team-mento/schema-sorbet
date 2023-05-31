package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"

	_ "embed"

	"github.com/carlmjohnson/versioninfo"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/iancoleman/strcase"
	"golang.org/x/exp/slices"
)

type Metadata struct {
	Command string
	Version string

	Modules []string

	Spec struct {
		Title   string
		Version string
	}
}

type Type struct {
	SchemaName           string
	TypeName             string
	Filename             string
	Type                 string
	Comment              string
	BaseClass            string
	Properties           []Property
	Enum                 []Enum
	Alias                string
	AdditionalProperties string

	IsArray bool
}

func (t Type) IsObject() bool {
	return "T::Struct" == t.BaseClass
}

func (t Type) IsEnum() bool {
	return len(t.Enum) > 0
}

type Property struct {
	Ref        string
	Name       string
	Type       string
	SchemaName string
	Required   bool
	// // JSONName is the :name property
	// JSONName string
}

type Enum struct {
	// Name contains the Ruby name for the enum value
	Name string
	// Value contains the value name as defined in the schema
	Value string
}

func (p *Property) RubyDefinition() string {
	s := fmt.Sprintf("const :%s, ", p.Name)

	if p.Required {
		s += p.Type
	} else {
		s += fmt.Sprintf("T.nilable(%s)", p.Type)
	}

	if p.SchemaName != p.Name {
		s += fmt.Sprintf(", name: '%s'", p.SchemaName)
	}

	return s
}

func prepareComment(s string) string {
	return strings.TrimSpace(s)
}

func parseString(name string, v *openapi3.SchemaRef) (types []Type) {
	t := Type{}
	t.SchemaName = name
	t.TypeName = strcase.ToCamel(name)
	t.Filename = strcase.ToSnake(name)
	t.Comment = prepareComment(v.Value.Description)
	t.Alias = "String"

	if v.Value.Enum != nil {
		if "string" != reflect.TypeOf(v.Value.Enum[0]).String() {
			log.Println("WARN: " + name + " has a non-string enum type (`  " + reflect.TypeOf(v.Value.Enum[0]).String() + " `), which may not work with enum generation")
		}
		for _, enum := range v.Value.Enum {
			val, ok := enum.(string)
			if !ok {
				log.Println("WARN: " + name + " has a non-string enum type (`  " + reflect.TypeOf(val).String() + " `), which failed to have its type converted to a string")
				continue
			}

			t.Enum = append(t.Enum, Enum{
				Name:  strcase.ToCamel(val),
				Value: val,
			})
		}
	}

	types = append(types, t)

	// TODO pattern
	// TODO format
	return types
}

func parseObject(name string, v *openapi3.SchemaRef) (types []Type) {
	t := Type{}
	t.SchemaName = name
	t.TypeName = strcase.ToCamel(name)
	t.Filename = strcase.ToSnake(name)
	t.Comment = prepareComment(v.Value.Description)
	t.BaseClass = "T::Struct"

	for propertyName, v2 := range v.Value.Properties {
		prop := Property{
			Name:       strcase.ToSnake(propertyName),
			SchemaName: propertyName,
			Type:       "T.untyped",
			Required:   slices.Contains(v.Value.Required, propertyName),
		}

		if v2.Ref != "" {
			parts := strings.Split(v2.Ref, "/")
			prop.Type = parts[len(parts)-1]
		} else {
			switch v2.Value.Type {
			case "string":
				prop.Type = "String"
			case "integer":
				prop.Type = "Integer"
			default:
				log.Printf("%s.%s had an unmatched v.Value.Type in parseObject: %#v\n", name, propertyName, v2.Value.Type)
			}
		}

		t.Properties = append(t.Properties, prop)
	}

	// ensure that we have consistent output
	slices.SortStableFunc(t.Properties, func(a, b Property) bool {
		return a.Name < b.Name
	})

	if v.Value.AdditionalProperties.Schema != nil {
		if v.Value.AdditionalProperties.Schema.Value.Type == "string" {
			t.AdditionalProperties = "String"
		} else {
			fmt.Printf("TODO: v.Value.AdditionalProperties.Schema.Value.Type: %v\n", v.Value.AdditionalProperties.Schema.Value.Type)
		}

	}

	types = append(types, t)

	return types
}

func parseSchema(name string, v *openapi3.SchemaRef) (types []Type) {
	if v.Ref != "" {
		fmt.Printf("Schema %s was a reference\n", name)
		return
	}

	switch v.Value.Type {
	case "string":
		types = append(types, parseString(name, v)...)
	case "object":
		types = append(types, parseObject(name, v)...)
	case "array":
		t := Type{}
		t.SchemaName = name
		t.TypeName = strcase.ToCamel(name)
		t.Filename = strcase.ToSnake(name)
		t.Comment = prepareComment(v.Value.Description)
		t.Alias = "T.untyped" // TODO this should be the type name, bu tit's unclear how to get it
		t.IsArray = true

		types = append(types, t)
	default:
		log.Printf("%s had an unmatched v.Value.Type in parseSchema: %#v\n", name, v.Value.Type)
	}

	return
}

func parseModules(module string) []string {
	modules := strings.Split(module, "::")
	if len(modules) == 1 && modules[0] == "" {
		modules = nil
	}

	return modules
}

func parseVersion() string {
	version := versioninfo.Short()
	if version == "" {
		version = "(unknown)"
	}
	return version
}

//go:embed class.rb.tmpl
var rawClassTemplate string

func main() {
	var path string
	var module string
	var out string
	flag.StringVar(&path, "path", "", "Path to OpenAPI document")
	flag.StringVar(&module, "module", "", "")
	flag.StringVar(&out, "out", "out", "")
	flag.Parse()

	doc, err := openapi3.NewLoader().LoadFromFile(path)
	must(err)

	classTemplate, err := template.New("").Funcs(template.FuncMap{}).Parse(rawClassTemplate)
	must(err)

	var allTypes []Type

	for k, v := range doc.Components.Schemas {
		types := parseSchema(k, v)
		if len(types) == 0 {
			log.Printf("Missing type data for schema %s\n", k)
		}
		allTypes = append(allTypes, types...)
	}

	modules := parseModules(module)

	// TODO
	outPathParts := []string{out}

	for _, m := range modules {
		outPathParts = append(outPathParts, strcase.ToSnake(m))
	}

	outPath := filepath.Join(outPathParts...)
	// TODO

	err = os.MkdirAll(outPath, os.ModePerm)
	must(err)

	metadata := Metadata{
		Command: "openapi-sorbet",
		Version: parseVersion(),

		Modules: modules,
	}
	metadata.Spec.Title = doc.Info.Title
	metadata.Spec.Version = doc.Info.Version

	for _, t := range allTypes {
		data := struct {
			Metadata Metadata
			Type     Type
		}{
			Metadata: metadata,
			Type:     t,
		}

		f, err := os.Create(filepath.Join(outPath, t.Filename) + ".rb")
		must(err)

		err = classTemplate.Execute(f, data)
		must(err)

		err = f.Close()
		must(err)
	}
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
