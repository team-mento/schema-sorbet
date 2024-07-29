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
	"github.com/iancoleman/strcase"
	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	"golang.org/x/exp/slices"
)

const (
	SorbetUntyped = "T.untyped"
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
	IsArray    bool
}

type Enum struct {
	// Name contains the Ruby name for the enum value
	Name string
	// Value contains the value name as defined in the schema
	Value string
}

func (p *Property) RubyDefinition() string {
	s := fmt.Sprintf("const :%s, ", p.Name)

	ty := p.Type
	if p.IsArray {
		ty = fmt.Sprintf("T::Array[%s]", ty)
	}

	if p.Required {
		s += ty
	} else {
		s += fmt.Sprintf("T.nilable(%s)", ty)
	}

	if p.SchemaName != p.Name {
		s += fmt.Sprintf(", name: '%s'", p.SchemaName)
	}

	return s
}

func prepareComment(s string) string {
	return strings.TrimSpace(s)
}

func parseString(name string, v *base.Schema) (types []Type) {
	t := Type{}
	t.SchemaName = name
	t.TypeName = strcase.ToCamel(name)
	t.Filename = strcase.ToSnake(name)
	t.Comment = prepareComment(v.Description)
	t.Alias = "String"

	if v.Enum != nil {
		if "string" != reflect.TypeOf(v.Enum[0]).String() {
			log.Println("WARN: " + name + " has a non-string enum type (`  " + reflect.TypeOf(v.Enum[0]).String() + " `), which may not work with enum generation")
		}
		for _, enum := range v.Enum {
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

func parseBoolean(name string, v *base.Schema) (types []Type) {
	t := Type{}
	t.SchemaName = name
	t.TypeName = strcase.ToCamel(name)
	t.Filename = strcase.ToSnake(name)
	t.Comment = prepareComment(v.Description)
	t.Alias = "T::Boolean"

	types = append(types, t)
	return
}

func parseObject(name string, v *base.Schema) (types []Type) {
	t := Type{}
	t.SchemaName = name
	t.TypeName = strcase.ToCamel(name)
	t.Filename = strcase.ToSnake(name)
	t.Comment = prepareComment(v.Description)
	t.BaseClass = "T::Struct"

	for propertyName, v2 := range v.Properties {
		prop := Property{
			Name:       strcase.ToSnake(propertyName),
			SchemaName: propertyName,
			Type:       SorbetUntyped,
			Required:   slices.Contains(v.Required, propertyName),
		}

		if v2.IsReference() {
			parts := strings.Split(v2.GetReference(), "/")
			prop.Type = strcase.ToCamel(parts[len(parts)-1])
		} else {
			schema := v2.Schema()
			if len(schema.Type) == 0 {
				log.Printf("Skipping property %s.%s as no Type was present", name, propertyName)
				continue
			}

			switch schema.Type[0] { //TODO
			case "string":
				prop.Type = "String"
			case "boolean":
				prop.Type = "T::Boolean"
			case "integer":
				prop.Type = "Integer"
			case "object":
				objectTypeName := name + "_" + propertyName

				childTypes := parseObject(objectTypeName, schema)
				types = append(types, childTypes...)

				prop.Type = strcase.ToCamel(objectTypeName)
			case "array":
				prop.IsArray = true
				prop.Type = SorbetUntyped

				if schema.Items.IsB() {
					// do nothing
				} else if schema.Items.IsA() {
					s := schema.Items.A
					if s.IsReference() {
						parts := strings.Split(s.GetReference(), "/")
						prop.Type = strcase.ToCamel(parts[len(parts)-1])
					} else {
						schema := s.Schema()
						if len(schema.Type) > 0 {
							switch schema.Type[0] { //TODO
							case "string":
								prop.Type = "String"
							case "integer":
								prop.Type = "Integer"
							default:
								log.Printf("%s had an unmatched v.Items.Schema.Type in parseObject: %#v\n", name, schema.Type[0])
							}
						} else {
							log.Printf("%s had an unset v.Items.Schema.Type in parseObject: %#v\n", name, schema.Type)
						}
					}
				} else {
					log.Printf("%s.%s had an unmatched v.Type in parseObject: %#v\n", name, propertyName, schema.Type[0])
				}
			default:
				log.Printf("%s.%s had an unmatched v.Type in parseObject: %#v\n", name, propertyName, schema.Type[0])
			}
		}

		t.Properties = append(t.Properties, prop)
	}

	// ensure that we have consistent output
	slices.SortStableFunc(t.Properties, func(a, b Property) bool {
		return a.Name < b.Name
	})

	if v.AdditionalProperties == true {
		t.AdditionalProperties = SorbetUntyped
	} else if v.AdditionalProperties != nil {
		sp, ok := v.AdditionalProperties.(*base.SchemaProxy)
		if ok {
			schema := sp.Schema()

			if len(schema.Type) > 0 {
				switch schema.Type[0] { //TODO
				case "string":
					t.AdditionalProperties = "String"
				case "integer":
					t.AdditionalProperties = "Integer"
				default:
					log.Printf("%s had an unmatched v.AdditionalProperties in parseObject: %#v\n", name, schema.Type[0])
				}
			} else {
				log.Printf("%s had an unmatched v.AdditionalProperties in parseObject: %#v\n", name, schema.Type)
			}
		}
	}

	types = append(types, t)

	return types
}

func parseArray(name string, v *base.Schema) (types []Type) {
	t := Type{}
	t.SchemaName = name
	t.TypeName = strcase.ToCamel(name)
	t.Filename = strcase.ToSnake(name)
	t.Comment = prepareComment(v.Description)
	t.Alias = SorbetUntyped
	t.IsArray = true

	// IsB here is whether this is an `items: true`
	if v.Items.IsB() {
		t.Alias = ""
		t.AdditionalProperties = SorbetUntyped
	} else if v.Items.IsA() {
		s := v.Items.A
		if s.IsReference() {
			parts := strings.Split(s.GetReference(), "/")
			t.Alias = parts[len(parts)-1]
		} else {
			schema := s.Schema()
			if len(schema.Type) > 0 {
				switch schema.Type[0] { //TODO
				case "string":
					t.Alias = "String"
				case "integer":
					t.Alias = "Integer"
				default:
					log.Printf("%s had an unmatched v.Items.Schema.Type in parseArray: %#v\n", name, schema.Type[0])
				}
			} else {
				log.Printf("%s had an unset v.Items.Schema.Type in parseArray: %#v\n", name, schema.Type)
			}
		}
	}

	types = append(types, t)

	return
}

func parseSchema(name string, v *base.Schema) (types []Type) {
	if len(v.Type) == 0 {
		log.Printf("Skipping %s as no Type was present", name)
		return
	}

	switch v.Type[0] { // TODO
	case "string":
		types = append(types, parseString(name, v)...)
	case "boolean":
		types = append(types, parseBoolean(name, v)...)
	case "object":
		types = append(types, parseObject(name, v)...)
	case "array":
		types = append(types, parseArray(name, v)...)
	default:
		log.Printf("%s had an unmatched v.Value.Type in parseSchema: %#v\n", name, v.Type)
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

	docBytes, err := os.ReadFile(path)
	must(err)

	document, err := libopenapi.NewDocument(docBytes)
	must(err)

	d, errors := document.BuildV3Model()
	if len(errors) > 0 {
		log.Printf("Failed to build OpenAPI v3 model for %s\n", path)
		for _, err2 := range errors {
			log.Println(err2)
		}
		log.Fatal("^^")
	}

	classTemplate, err := template.New("").Funcs(template.FuncMap{}).Parse(rawClassTemplate)
	must(err)

	var allTypes []Type

	for k, sp := range d.Model.Components.Schemas {
		if sp.IsReference() {
			log.Printf("Skipping %s as ref", k)
			continue
		}

		schema := sp.Schema()
		types := parseSchema(k, schema)
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
	metadata.Spec.Title = d.Model.Info.Title
	metadata.Spec.Version = d.Model.Info.Version

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
