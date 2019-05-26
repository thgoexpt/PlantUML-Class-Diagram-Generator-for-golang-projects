/*
Package parser generates PlantUml http://plantuml.com/ Class diagrams for your golang projects
The main structure is the ClassParser which you can generate by calling the NewClassDiagram(dir)
function.

Pass the directory where the .go files are and the parser will analyze the code and build a structure
containing the information it needs to Render the class diagram.

call teh Render() function and this will return a string with the class diagram.

See github.com/jfeliu007/goplantuml/cmd/goplantuml/main.go for a command that uses this functions and outputs the text to
the console.

*/
package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"unicode"
)

//LineStringBuilder extends the strings.Builder and adds functionality to build a string with tabs and
//adding new lines
type LineStringBuilder struct {
	strings.Builder
}

const tab = "    "

//WriteLineWithDepth will write the given text with added tabs at the begining into the string builder.
func (lsb *LineStringBuilder) WriteLineWithDepth(depth int, str string) {
	lsb.WriteString(strings.Repeat(tab, depth))
	lsb.WriteString(str)
	lsb.WriteString("\n")
}

//ClassParser contains the structure of the parsed files. The structure is a map of package_names that contains
//a map of structure_names -> Structs
type ClassParser struct {
	structure          map[string]map[string]*Struct
	currentPackageName string
	allInterfaces      map[string]struct{}
	allStructs         map[string]struct{}
}

//NewClassDiagram returns a new classParser with which can Render the class diagram of
// files int eh given directory
func NewClassDiagram(directoryPath string) (*ClassParser, error) {
	classParser := &ClassParser{
		structure:     make(map[string]map[string]*Struct),
		allInterfaces: make(map[string]struct{}),
		allStructs:    make(map[string]struct{}),
	}
	fs := token.NewFileSet()
	result, err := parser.ParseDir(fs, directoryPath, nil, 0)
	if err != nil {
		return nil, err
	}
	for _, v := range result {
		classParser.parsePackage(v)
	}
	for s := range classParser.allStructs {
		st := classParser.getStruct(s)
		if st != nil {
			for i := range classParser.allInterfaces {
				inter := classParser.getStruct(i)
				if st.ImplementsInterface(inter) {
					st.AddToExtends(i)
				}
			}
		}
	}
	return classParser, nil
}

//parse the given ast.Package into the ClassParser structure
func (p *ClassParser) parsePackage(node ast.Node) {
	pack := node.(*ast.Package)
	p.currentPackageName = pack.Name
	_, ok := p.structure[p.currentPackageName]
	if !ok {
		p.structure[p.currentPackageName] = make(map[string]*Struct)
	}
	for fileName, f := range pack.Files {
		if !strings.HasSuffix(fileName, "_test.go") {
			for _, d := range f.Decls {
				p.parseFileDeclarations(d)
			}
		}
	}
}

//parse the given declaration looking for classes, interfaces, or member functions
func (p *ClassParser) parseFileDeclarations(node ast.Decl) {
	switch decl := node.(type) {
	case *ast.GenDecl:
		spec := decl.Specs[0]
		var declarationType string
		var typeName string
		switch v := spec.(type) {
		case *ast.TypeSpec:
			typeName = v.Name.Name
			switch c := v.Type.(type) {
			case *ast.StructType:
				declarationType = "class"
				for _, f := range c.Fields.List {
					p.getOrCreateStruct(typeName).AddField(f)
				}
				break
			case *ast.InterfaceType:
				declarationType = "interface"
				for _, f := range c.Methods.List {
					p.getOrCreateStruct(typeName).AddMethod(f)
				}
				break
			default:
				// Not needed for class diagrams (Imports, global variables, regular functions, etc)
				return
			}
		default:
			// Not needed for class diagrams (Imports, global variables, regular functions, etc)
			return
		}
		p.getOrCreateStruct(typeName).Type = declarationType
		fullName := fmt.Sprintf("%s.%s", p.currentPackageName, typeName)
		switch declarationType {
		case "interface":
			p.allInterfaces[fullName] = struct{}{}
			break
		case "class":
			p.allStructs[fullName] = struct{}{}
			break
		}
		break
	case *ast.FuncDecl:
		if decl.Recv != nil {
			// Only get in when the function is defined for a structure. Global functions are not needed for class diagram
			theType := getFieldType(decl.Recv.List[0].Type, "")
			if theType[0] == "*"[0] {
				theType = theType[1:]
			}
			structure := p.getOrCreateStruct(theType)
			if structure.Type == "" {
				structure.Type = "class"
			}
			structure.AddMethod(&ast.Field{
				Names:   []*ast.Ident{decl.Name},
				Doc:     decl.Doc,
				Type:    decl.Type,
				Tag:     nil,
				Comment: nil,
			})
		}
		break
	}
}

//Render returns a string of the class diagram that this parser has generated.
func (p *ClassParser) Render() string {
	str := &LineStringBuilder{}
	str.WriteLineWithDepth(0, "@startuml")
	for pack, structures := range p.structure {
		composition := &LineStringBuilder{}
		extends := &LineStringBuilder{}
		if len(structures) > 0 {
			str.WriteLineWithDepth(0, fmt.Sprintf(`namespace %s {`, pack))
			for name, structure := range structures {
				privateFields := &LineStringBuilder{}
				publicFields := &LineStringBuilder{}
				privateMethods := &LineStringBuilder{}
				publicMethods := &LineStringBuilder{}
				str.WriteLineWithDepth(1, fmt.Sprintf(`%s %s {`, structure.Type, name))
				for _, field := range structure.Fields {
					accessModifier := "+"
					if unicode.IsLower(rune(field.Name[0])) {
						accessModifier = "-"
					}
					if accessModifier == "-" {
						privateFields.WriteLineWithDepth(2, fmt.Sprintf(`%s %s %s`, accessModifier, field.Name, field.Type))
					} else {
						publicFields.WriteLineWithDepth(2, fmt.Sprintf(`%s %s %s`, accessModifier, field.Name, field.Type))
					}
				}
				for _, c := range structure.Composition {
					if !strings.Contains(c, ".") {
						c = fmt.Sprintf("%s.%s", structure.PackageName, c)
					}
					composition.WriteLineWithDepth(0, fmt.Sprintf(`%s *-- %s.%s`, c, pack, name))
				}
				for _, c := range structure.Extends {
					if !strings.Contains(c, ".") {
						c = fmt.Sprintf("%s.%s", structure.PackageName, c)
					}
					extends.WriteLineWithDepth(0, fmt.Sprintf(`%s <|-- %s.%s`, c, pack, name))
				}
				for _, method := range structure.Functions {
					accessModifier := "+"
					if unicode.IsLower(rune(method.Name[0])) {
						accessModifier = "-"
					}
					parameterList := make([]string, 0)
					for _, p := range method.Parameters {
						parameterList = append(parameterList, fmt.Sprintf("%s %s", p.Name, p.Type))
					}
					returnValues := ""
					if len(method.ReturnValues) > 1 {
						returnValues = fmt.Sprintf("(%s)", strings.Join(method.ReturnValues, ", "))
					}
					if accessModifier == "-" {
						privateMethods.WriteLineWithDepth(2, fmt.Sprintf(`%s %s(%s) %s`, accessModifier, method.Name, strings.Join(parameterList, ", "), returnValues))
					} else {
						publicMethods.WriteLineWithDepth(2, fmt.Sprintf(`%s %s(%s) %s`, accessModifier, method.Name, strings.Join(parameterList, ", "), returnValues))
					}
				}
				if privateFields.Len() > 0 {
					str.WriteLineWithDepth(0, privateFields.String())
				}
				if publicFields.Len() > 0 {
					str.WriteLineWithDepth(0, publicFields.String())
				}
				if privateMethods.Len() > 0 {
					str.WriteLineWithDepth(0, privateMethods.String())
				}
				if publicMethods.Len() > 0 {
					str.WriteLineWithDepth(0, publicMethods.String())
				}
				str.WriteLineWithDepth(1, fmt.Sprintf(`}`))
			}
			str.WriteLineWithDepth(0, fmt.Sprintf(`}`))
			str.WriteLineWithDepth(0, composition.String())
			str.WriteLineWithDepth(0, extends.String())
		}

	}
	str.WriteString("@enduml")
	return str.String()
}

// Returns an initialized struct of the given name or returns the existing one if it was already created
func (p *ClassParser) getOrCreateStruct(name string) *Struct {
	result, ok := p.structure[p.currentPackageName][name]
	if !ok {
		result = &Struct{
			PackageName: p.currentPackageName,
			Functions:   make([]*Function, 0),
			Fields:      make([]*Field, 0),
			Type:        "",
			Composition: make([]string, 0),
			Extends:     make([]string, 0),
		}
		p.structure[p.currentPackageName][name] = result
	}
	return result
}

// Returns an existing struct only if it was created. nil otherwhise
func (p *ClassParser) getStruct(structName string) *Struct {
	split := strings.SplitN(structName, ".", 2)
	pack, ok := p.structure[split[0]]
	if !ok {
		return nil
	}
	return pack[split[1]]
}
