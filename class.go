package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
)

type ConstantPoolItem interface {
	isConstantPoolItem()
	String() string
}

type accessFlags uint16

const (
	Public     accessFlags = 0x0001
	Static                 = 0x0008
	Final                  = 0x0010
	Super                  = 0x0020
	Native                 = 0x0100
	Interface              = 0x0200
	Abstract               = 0x0400
	Synthetic              = 0x1000
	Annotation             = 0x2000
	Enum                   = 0x4000
)

type Code struct {
	maxStack          uint16
	maxLocals         uint16
	Instructions      []byte
	ExceptionHandlers []ExceptionHandler
}

type Class struct {
	magic             uint32
	MinorVersion      uint16
	MajorVersion      uint16
	ConstantPoolItems []ConstantPoolItem
	AccessFlags       accessFlags
	thisClass         uint16
	superClass        uint16
	interfaces        []uint16
	fields            []field
	methods           []Method
	initialised       bool
}

type ExceptionHandler struct {
	Start     uint16
	End       uint16
	Handler   uint16
	CatchType uint16
	Class     string
}

func parseCode(cr byteParser, length uint32, method *Method) {
	var c Code
	c.maxStack = cr.u2()
	c.maxLocals = cr.u2()
	codeLength := cr.u4()
	c.Instructions = make([]byte, codeLength)
	for k := 0; k < len(c.Instructions); k++ {
		c.Instructions[k] = cr.u1()
	}
	numExceptionHandlers := cr.u2()
	c.ExceptionHandlers = make([]ExceptionHandler, numExceptionHandlers)
	for i := 0; i < len(c.ExceptionHandlers); i++ {
		c.ExceptionHandlers[i].Start = cr.u2()
		c.ExceptionHandlers[i].End = cr.u2()
		c.ExceptionHandlers[i].Handler = cr.u2()
		catchType := cr.u2()
		if catchType != 0 {
			c.ExceptionHandlers[i].CatchType = catchType
			info := method.class.ConstantPoolItems[catchType-1].(classInfo)
			name := method.class.ConstantPoolItems[info.nameIndex-1].(utf8String)
			c.ExceptionHandlers[i].Class = name.contents
		}
	}
	for k := uint32(8) + codeLength + 2 + uint32(numExceptionHandlers)*8; k < length; k++ {
		_ = cr.u1()
	}
	method.Code = c
}

type byteParser struct {
	reader io.Reader
	err    error
}

func newByteParser(byteSlice []byte, start int) byteParser {
	return byteParser{
		reader: bytes.NewReader(byteSlice[start:]),
	}
}

func (r byteParser) u8() uint64 {
	if r.err != nil {
		return 0
	}
	var x uint64
	r.err = binary.Read(r.reader, binary.BigEndian, &x)
	return x
}

func (r byteParser) u4() uint32 {
	if r.err != nil {
		return 0
	}
	var x uint32
	r.err = binary.Read(r.reader, binary.BigEndian, &x)
	return x
}

func (r byteParser) u2() uint16 {
	if r.err != nil {
		return 0
	}
	var x uint16
	r.err = binary.Read(r.reader, binary.BigEndian, &x)
	return x
}

func (r byteParser) u1() uint8 {
	if r.err != nil {
		return 0
	}
	var x uint8
	r.err = binary.Read(r.reader, binary.BigEndian, &x)
	return x
}

func newClassDecoder(r io.Reader) byteParser {
	cr := byteParser{r, nil}
	magic := cr.u4()
	if magic != 0xCAFEBABE {
		cr.err = errors.New("Bad magic number")
	}
	return cr
}

func ParseClass(r io.Reader) (c Class, err error) {
	cr := newClassDecoder(r)
	c.MinorVersion = cr.u2() // minor version
	c.MajorVersion = cr.u2() // major version
	cpc := cr.u2()
	//constantPoolCount := cpc - 1
	if cpc != 0 {
		//c.ConstantPoolItems = parseConstantPool(&c, cr, constantPoolCount)
	}

	c.AccessFlags = accessFlags(cr.u2())
	c.thisClass = cr.u2()
	c.superClass = cr.u2()

	interfacesCount := cr.u2()
	c.interfaces = make([]uint16, interfacesCount)
	for i := uint16(0); i < interfacesCount; i++ {
		c.interfaces[i] = cr.u2()
	}

	fieldsCount := cr.u2()
	c.fields = make([]field, fieldsCount)
	for i := uint16(0); i < fieldsCount; i++ {
		c.fields[i].accessFlags = accessFlags(cr.u2())
		c.fields[i].nameIndex = cr.u2()
		c.fields[i].descriptorIndex = cr.u2()

		attrCount := cr.u2()
		for j := uint16(0); j < attrCount; j++ {
			_ = cr.u2()
			length := cr.u4()
			for k := uint32(0); k < length; k++ {
				_ = cr.u1() // throw away bytes
			}
		}
	}

	methodsCount := cr.u2()
	c.methods = make([]Method, methodsCount)
	for i := uint16(0); i < methodsCount; i++ {
		c.methods[i].class = &c
		c.methods[i].accessFlags = accessFlags(cr.u2())
		c.methods[i].nameIndex = cr.u2()
		c.methods[i].descriptorIndex = cr.u2()

		var sig string
		sig = c.ConstantPoolItems[c.methods[i].descriptorIndex-1].(utf8String).contents
		c.methods[i].Signiture = parseSigniture(sig)
		c.methods[i].RawSigniture = sig

		attrCount := cr.u2()
		for j := uint16(0); j < attrCount; j++ {
			name := cr.u2()
			length := cr.u4()
			actualName := (c.ConstantPoolItems[name-1]).(utf8String)
			if actualName.contents == "Code" {
				parseCode(cr, length, &c.methods[i])
			} else {
				for k := uint32(0); k < length; k++ {
					_ = cr.u1() // throw away bytes
				}
			}
		}
	}
	attrCount := cr.u2()
	for j := uint16(0); j < attrCount; j++ {
		_ = cr.u2()
		length := cr.u4()
		for k := uint32(0); k < length; k++ {
			_ = cr.u1() // throw away bytes
		}
	}

	return c, cr.err
}

func (c *Class) hasMethodCalled(name string) bool {
	for _, m := range c.methods {
		n := c.ConstantPoolItems[m.nameIndex-1].(utf8String).contents
		if n == name {
			return true
		}
	}
	return false
}

func (c *Class) resolveMethod(name string, descriptor string) (*Method, []string) {
	var avaiableMethods []string
	for i, m := range c.methods {
		n := c.ConstantPoolItems[m.nameIndex-1].(utf8String).contents
		d := c.ConstantPoolItems[m.descriptorIndex-1].(utf8String).contents
		//log.Printf("Comparing %v::%v to %v::%v\n", name, descriptor, n, d)
		avaiableMethods = append(avaiableMethods, fmt.Sprintf("%s::%s%s", c.Name(), n, d))
		if n == name && d == descriptor {
			return &c.methods[i], avaiableMethods
		}
	}
	return nil, avaiableMethods
}

func (c *Class) getField(name string) *field {
	for i, f := range c.fields {
		n := c.ConstantPoolItems[f.nameIndex-1].(utf8String).contents
		if n == name {
			return &(c.fields[i])
		}
	}
	panic(fmt.Sprintf("Could not find field called %v", name))
}

func (c *Class) Name() string {
	info := c.ConstantPoolItems[c.thisClass-1].(classInfo)
	name := c.ConstantPoolItems[info.nameIndex-1].(utf8String)
	return name.contents
}

func (c *Class) getSuperName() string {
	info := c.ConstantPoolItems[c.superClass-1].(classInfo)
	name := c.ConstantPoolItems[info.nameIndex-1].(utf8String)
	return name.contents
}

func (c *Class) getMethodRefAt(index uint16) methodRef {
	return c.ConstantPoolItems[index-1].(methodRef)
}

func (c *Class) getInterfaceMethodRefAt(index uint16) interfaceMethodRef {
	return c.ConstantPoolItems[index-1].(interfaceMethodRef)
}

func (c *Class) getFieldRefAt(index uint16) fieldRef {
	return c.ConstantPoolItems[index-1].(fieldRef)
}

func (c *Class) getClassInfoAt(index uint16) classInfo {
	return c.ConstantPoolItems[index-1].(classInfo)
}

func (c *Class) getConstantPoolItemAt(index uint16) ConstantPoolItem {
	return c.ConstantPoolItems[index-1]
}

func (c *Class) getStringAt(index int) utf8String {
	strRef := c.ConstantPoolItems[index].(stringConstant)
	return c.ConstantPoolItems[strRef.utf8Index-1].(utf8String)
}

func (c *Class) getLongAt(index int) longConstant {
	return c.ConstantPoolItems[index].(longConstant)
}

func (m methodRef) methodName() string {
	nt := m.containingClass.ConstantPoolItems[m.nameAndTypeIndex-1].(nameAndType)
	n := m.containingClass.ConstantPoolItems[nt.nameIndex-1].(utf8String).contents
	return n
}

func (m methodRef) className() string {
	ct := m.containingClass.ConstantPoolItems[m.classIndex-1].(classInfo)
	c := m.containingClass.ConstantPoolItems[ct.nameIndex-1].(utf8String).contents
	return c
}

func (m methodRef) methodType() string {
	nt := m.containingClass.ConstantPoolItems[m.nameAndTypeIndex-1].(nameAndType)
	t := m.containingClass.ConstantPoolItems[nt.descriptorIndex-1].(utf8String).contents
	return t
}

func (m interfaceMethodRef) methodName() string {
	nt := m.containingClass.ConstantPoolItems[m.nameAndTypeIndex-1].(nameAndType)
	n := m.containingClass.ConstantPoolItems[nt.nameIndex-1].(utf8String).contents
	return n
}

func (m interfaceMethodRef) methodType() string {
	nt := m.containingClass.ConstantPoolItems[m.nameAndTypeIndex-1].(nameAndType)
	t := m.containingClass.ConstantPoolItems[nt.descriptorIndex-1].(utf8String).contents
	return t
}

func (m interfaceMethodRef) className() string {
	ct := m.containingClass.ConstantPoolItems[m.classIndex-1].(classInfo)
	c := m.containingClass.ConstantPoolItems[ct.nameIndex-1].(utf8String).contents
	return c
}

func (ct classInfo) className() string {
	c := ct.containingClass.ConstantPoolItems[ct.nameIndex-1].(utf8String).contents
	return c
}

func (m fieldRef) fieldName() string {
	nt := m.containingClass.ConstantPoolItems[m.nameAndTypeIndex-1].(nameAndType)
	n := m.containingClass.ConstantPoolItems[nt.nameIndex-1].(utf8String).contents
	return n
}

func (m fieldRef) fieldDescriptor() string {
	nt := m.containingClass.ConstantPoolItems[m.nameAndTypeIndex-1].(nameAndType)
	t := m.containingClass.ConstantPoolItems[nt.descriptorIndex-1].(utf8String).contents
	return t
}

func (m fieldRef) className() string {
	ct := m.containingClass.ConstantPoolItems[m.classIndex-1].(classInfo)
	c := m.containingClass.ConstantPoolItems[ct.nameIndex-1].(utf8String).contents
	return c
}

func parseSigniture(sig string) []string {
	s := make([]string, 0)
	className := false
	for _, c := range sig {
		//TODO: save the name of the class. Maybe
		if className {
			if c == ';' {
				className = false
			}
			continue
		}
		switch c {
		case '(', ')', '[':
			break
		case 'B':
			s = append(s, "byte")
			break
		case 'C':
			s = append(s, "char")
			break
		case 'D':
			s = append(s, "double")
			break
		case 'F':
			s = append(s, "float")
			break
		case 'I':
			s = append(s, "int")
			break
		case 'J':
			s = append(s, "long")
			break
		case 'S':
			s = append(s, "short")
			break
		case 'Z':
			s = append(s, "boolean")
			break
		case 'V':
			s = append(s, "void")
			break
		case 'L':
			s = append(s, "reference")
			className = true
			break
		default:
			log.Panicf("Can't parse signiture: %s", sig)
		}
	}
	return s
}

type methodType struct {
	descriptorIndex uint16
}

func (_ methodType) isConstantPoolItem() {}

func (n methodType) String() string {
	return fmt.Sprintf("(MethodType)")
}

func parseMethodType(c *Class, cr byteParser) ConstantPoolItem {
	return methodType{cr.u2()}
}

type methodHandle struct {
	referenceKind  uint8
	referenceIndex uint16
}

func (_ methodHandle) isConstantPoolItem() {}

func (n methodHandle) String() string {
	return fmt.Sprintf("(MethodHandle)")
}

func parseMethodHandle(c *Class, cr byteParser) ConstantPoolItem {
	return methodHandle{cr.u1(), cr.u2()}
}

type invokeDynamic struct {
	bootstrapMethodAttrIndex uint16
	nameAndTypeIndex         uint16
}

func (_ invokeDynamic) isConstantPoolItem() {}

func (n invokeDynamic) String() string {
	return fmt.Sprintf("(InvokeDynamic) bootstrapMethodAttrIndex: %d, nameAndType: %d", n.bootstrapMethodAttrIndex, n.nameAndTypeIndex)
}

func parseInvokeDynamic(c *Class, cr byteParser) ConstantPoolItem {
	return invokeDynamic{cr.u2(), cr.u2()}
}

type nameAndType struct {
	nameIndex       uint16
	descriptorIndex uint16
}

func (_ nameAndType) isConstantPoolItem() {}

func (n nameAndType) String() string {
	return fmt.Sprintf("(NameAndType) name: %d, type: %d", n.nameIndex, n.descriptorIndex)
}

func parseNameAndType(c *Class, cr byteParser) ConstantPoolItem {
	nameIndex := cr.u2()
	descriptorIndex := cr.u2()
	return nameAndType{nameIndex, descriptorIndex}
}

type utf8String struct {
	contents string
}

func (_ utf8String) isConstantPoolItem() {}

func (u utf8String) String() string {
	return "(String) \"" + u.contents + "\""
}

func parseUTF8String(c *Class, cr byteParser) ConstantPoolItem {
	length := cr.u2()
	bytes := make([]byte, length)
	for i := uint16(0); i < length; i++ {
		bytes[i] = cr.u1()
	}
	return utf8String{string(bytes)}
}

type classInfo struct {
	containingClass *Class
	nameIndex       uint16
}

func (_ classInfo) isConstantPoolItem() {}

func (c classInfo) String() string {
	return fmt.Sprintf("(ClassInfo) %d", c.nameIndex)
}

func parseClassInfo(c *Class, cr byteParser) ConstantPoolItem {
	nameIndex := cr.u2()
	return classInfo{c, nameIndex}
}

type methodRef struct {
	containingClass  *Class
	classIndex       uint16
	nameAndTypeIndex uint16
}

func (_ methodRef) isConstantPoolItem() {}

func (m methodRef) String() string {
	return fmt.Sprintf("(MethodRef) class: %d, name: %d", m.classIndex, m.nameAndTypeIndex)
}

func parseMethodRef(c *Class, cr byteParser) ConstantPoolItem {
	classIndex := cr.u2()
	nameAndTypeIndex := cr.u2()
	return methodRef{c, classIndex, nameAndTypeIndex}
}

type interfaceMethodRef struct {
	containingClass  *Class
	classIndex       uint16
	nameAndTypeIndex uint16
}

func (_ interfaceMethodRef) isConstantPoolItem() {}

func (i interfaceMethodRef) String() string {
	return fmt.Sprintf("(InterfaceMethodRef) class: %d, name: %d", i.classIndex, i.nameAndTypeIndex)
}

func parseInterfaceMethodRef(c *Class, cr byteParser) ConstantPoolItem {
	classIndex := cr.u2()
	nameAndTypeIndex := cr.u2()
	return interfaceMethodRef{c, classIndex, nameAndTypeIndex}
}

type fieldRef struct {
	containingClass  *Class
	classIndex       uint16
	nameAndTypeIndex uint16
}

func (_ fieldRef) isConstantPoolItem() {}

func (f fieldRef) String() string {
	return fmt.Sprintf("(FieldRef) class: %d, name %d", f.classIndex, f.nameAndTypeIndex)
}

func parseFieldRef(c *Class, cr byteParser) ConstantPoolItem {
	classIndex := cr.u2()
	nameAndTypeIndex := cr.u2()
	return fieldRef{c, classIndex, nameAndTypeIndex}
}

type stringConstant struct {
	utf8Index uint16
}

func (_ stringConstant) isConstantPoolItem() {}

func (s stringConstant) String() string {
	return fmt.Sprintf("(StringConst) index: %d", s.utf8Index)
}

func parseStringConstant(c *Class, cr byteParser) ConstantPoolItem {
	utf8Index := cr.u2()
	return stringConstant{utf8Index}
}

type intConstant struct {
	value int32
}

func (_ intConstant) isConstantPoolItem() {}

func (i intConstant) String() string {
	return fmt.Sprintf("(Int) %d", i.value)
}

func parseIntConstant(c *Class, cr byteParser) ConstantPoolItem {
	i := int32(cr.u4())
	return intConstant{i}
}

type longConstant struct {
	value int64
}

func (_ longConstant) isConstantPoolItem() {}

func (l longConstant) String() string {
	return fmt.Sprintf("(Long) %d", l.value)
}

func parseLongConstant(c *Class, cr byteParser) ConstantPoolItem {
	long := int64(cr.u4()) << 32
	long += int64(cr.u4())
	return longConstant{long}
}

type WideConstantPart2 struct {
}

func (_ WideConstantPart2) isConstantPoolItem() {}

func (l WideConstantPart2) String() string {
	return fmt.Sprintf("(Long Part 2)")
}

type floatConstant struct {
	value float32
}

func (_ floatConstant) isConstantPoolItem() {}

func (f floatConstant) String() string {
	return fmt.Sprintf("(Float) %f", f.value)
}

func parseFloatConstant(c *Class, cr byteParser) ConstantPoolItem {
	bits := cr.u4()
	return floatConstant{math.Float32frombits(bits)}
}

type doubleConstant struct {
	value float64
}

func (_ doubleConstant) isConstantPoolItem() {}

func (f doubleConstant) String() string {
	return fmt.Sprintf("(Double) %v", f.value)
}

func parseDoubleConstant(c *Class, cr byteParser) ConstantPoolItem {
	bits := cr.u8()
	return doubleConstant{math.Float64frombits(bits)}
}

type field struct {
	accessFlags     accessFlags
	nameIndex       uint16
	descriptorIndex uint16
	value           interface{}
}

type Method struct {
	class           *Class
	Signiture       []string
	RawSigniture    string
	accessFlags     accessFlags
	nameIndex       uint16
	descriptorIndex uint16
	Code            Code
}

func (m *Method) Name() string {
	return m.class.ConstantPoolItems[m.nameIndex-1].(utf8String).contents
}

func (m *Method) Class() *Class {
	return m.class
}

func (m *Method) Static() bool {
	return m.accessFlags&Static != 0
}

func (m *Method) Native() bool {
	return m.accessFlags&Native != 0
}

func (m *Method) numArgs() int {
	return len(m.Signiture) - 1
}

func (m *Method) Sig() []string {
	return m.Signiture
}

var globalId int

func nextId() (id int) {
	id = globalId
	globalId++
	return
}

func parseMagicNumber(bytes []byte, index int) (next int, section *Section) {
	next = index
	if len(bytes) >= 4 {
		magic := newByteParser(bytes, index).u4()
		if magic == 0xCAFEBABE {
			next += 4
			section = &Section{
				Id:         nextId(),
				StartIndex: index,
				EndIndex:   next,
				Name:       "magic number",
			}
		}
	}
	return
}

func parseVersion(bytes []byte, index int) (next int, section *Section) {
	next = index
	if len(bytes) >= 4 {
		parser := newByteParser(bytes, index)
		minorVersion := parser.u2()
		majorVersion := parser.u2()
		if parser.err == nil {
			next += 4
			minorVersionSection := Section{
				Id:         nextId(),
				StartIndex: index,
				EndIndex:   index + 2,
				Name:       fmt.Sprintf("minor version: %d", minorVersion),
			}
			majorVersionSection := Section{
				Id:         nextId(),
				StartIndex: index + 2,
				EndIndex:   index + 4,
				Name:       fmt.Sprintf("major version: %d", majorVersion),
			}
			section = &Section{
				Id:         nextId(),
				StartIndex: index,
				EndIndex:   next,
				Name:       fmt.Sprintf("version %d.%d", majorVersion, minorVersion),
				Children:   []Section{minorVersionSection, majorVersionSection},
			}
		}
	}
	return
}

func parse(bytes []byte, index int) (next int, section *Section) {
	next = index
	return
}

func parseInterfaces(bytes []byte, index int) (next int, section *Section) {
	next = index + 2
	parser := newByteParser(bytes, index)
	interfacesCount := int(parser.u2())
	var children []Section
	for i := 0; i < interfacesCount; i++ {
		children = append(children, Section{
			Id:         nextId(),
			StartIndex: next,
			EndIndex:   next + 2,
			Name:       fmt.Sprintf("constant pool index for interface: ", parser.u2()),
		})
		next += 2
	}
	section = &Section{
		Id:         nextId(),
		StartIndex: index,
		EndIndex:   next,
		Name:       fmt.Sprintf("class implements %v interfaces", interfacesCount),
		Children:   children,
	}
	return
}

func parseThisClass(bytes []byte, index int) (next int, section *Section) {
	next = index + 2
	parser := newByteParser(bytes, index)
	this := parser.u2()
	section = &Section{
		Id:         nextId(),
		StartIndex: index,
		EndIndex:   next,
		Name:       fmt.Sprintf("constant pool index for this class: %v", this),
	}
	return
}

func parseSuperClass(bytes []byte, index int) (next int, section *Section) {
	next = index + 2
	parser := newByteParser(bytes, index)
	super := parser.u2()
	section = &Section{
		Id:         nextId(),
		StartIndex: index,
		EndIndex:   next,
		Name:       fmt.Sprintf("constant pool index for super class: %v", super),
	}
	return
}

func parseAccessFlags(bytes []byte, index int) (next int, section *Section) {
	next = index + 2
	parser := newByteParser(bytes, index)
	flags := accessFlags(parser.u2())
	publicSec := Section{
		Id:         nextId(),
		StartIndex: index + 1,
		EndIndex:   index + 2,
		Name:       fmt.Sprintf("0x0001 public: %v", flags&Public != 0),
	}
	staticSec := Section{
		Id:         nextId(),
		StartIndex: index + 1,
		EndIndex:   index + 2,
		Name:       fmt.Sprintf("0x0008 static: %v", flags&Static != 0),
	}
	finalSec := Section{
		Id:         nextId(),
		StartIndex: index + 1,
		EndIndex:   index + 2,
		Name:       fmt.Sprintf("0x0010 final: %v", flags&Final != 0),
	}
	superSec := Section{
		Id:         nextId(),
		StartIndex: index + 1,
		EndIndex:   index + 2,
		Name:       fmt.Sprintf("0x0020 super: %v", flags&Super != 0),
	}
	nativeSec := Section{
		Id:         nextId(),
		StartIndex: index,
		EndIndex:   index + 1,
		Name:       fmt.Sprintf("0x0100 native: %v", flags&Native != 0),
	}
	interfaceSec := Section{
		Id:         nextId(),
		StartIndex: index,
		EndIndex:   index + 1,
		Name:       fmt.Sprintf("0x0200 interface: %v", flags&Interface != 0),
	}
	abstractSec := Section{
		Id:         nextId(),
		StartIndex: index,
		EndIndex:   index + 1,
		Name:       fmt.Sprintf("0x0400 abstract: %v", flags&Abstract != 0),
	}
	syntheticSec := Section{
		Id:         nextId(),
		StartIndex: index,
		EndIndex:   index + 1,
		Name:       fmt.Sprintf("0x1000 synthetic: %v", flags&Synthetic != 0),
	}
	annotationSec := Section{
		Id:         nextId(),
		StartIndex: index,
		EndIndex:   index + 1,
		Name:       fmt.Sprintf("0x2000 annotation: %v", flags&Annotation != 0),
	}
	enumSec := Section{
		Id:         nextId(),
		StartIndex: index,
		EndIndex:   index + 1,
		Name:       fmt.Sprintf("0x4000 enum: %v", flags&Enum != 0),
	}
	section = &Section{
		Id:         nextId(),
		StartIndex: index,
		EndIndex:   index + 2,
		Name:       "access flags",
		Children: []Section{
			publicSec,
			staticSec,
			finalSec,
			superSec,
			nativeSec,
			interfaceSec,
			abstractSec,
			syntheticSec,
			annotationSec,
			enumSec,
		},
	}
	return
}

func parseConstantPool(bytes []byte, index int) (next int, section *Section) {
	next = index
	parser := newByteParser(bytes, index)
	constantPoolCount := parser.u2()
	next += 2
	var children []Section
	children = append(children, Section{
		Id:         nextId(),
		StartIndex: index,
		EndIndex:   next,
		Name:       fmt.Sprintf("constant pool count: %d", constantPoolCount),
	})

loop:
	for i := 0; i < int(constantPoolCount-1); i++ {
		var item Section
		item.Id = nextId()
		var tagSec Section
		tagSec.Id = nextId()
		item.StartIndex = next
		tag := parser.u1()
		tagSec.StartIndex = next
		next++
		tagSec.EndIndex = next
		switch tag {
		case 1:
			item.Name = fmt.Sprintf("[%d] UTF-8 string", i+1)
			tagSec.Name = fmt.Sprintf("tag: %d", tag)
			length := parser.u2()
			strBytes := make([]byte, length)
			for i := uint16(0); i < length; i++ {
				strBytes[i] = parser.u1()
			}
			item.Children = append(item.Children, tagSec)
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("length: %d", length),
			})
			next += 2
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + int(length),
				Name:       fmt.Sprintf("string: \"%s\"", string(strBytes)),
			})
			next += int(length)
		case 3:
			item.Name = fmt.Sprintf("[%d] int", i+1)
			tagSec.Name = fmt.Sprintf("tag: %d", tag)
			item.Children = append(item.Children, tagSec)
			x := int32(parser.u4())
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 4,
				Name:       fmt.Sprintf("%d", x),
			})
			next += 4
		case 4:
			item.Name = fmt.Sprintf("[%d] float", i+1)
			tagSec.Name = fmt.Sprintf("tag: %d", tag)
			item.Children = append(item.Children, tagSec)
			bits := parser.u4()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 4,
				Name:       fmt.Sprintf("%v", math.Float32frombits(bits)),
			})
			next += 4
		case 5:
			item.Name = fmt.Sprintf("[%d] long", i+1)
			tagSec.Name = fmt.Sprintf("tag: %d", tag)
			item.Children = append(item.Children, tagSec)
			x := parser.u8()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 8,
				Name:       fmt.Sprintf("%v", x),
			})
			next += 8
			i++
		case 6:
			item.Name = fmt.Sprintf("[%d] double", i+1)
			tagSec.Name = fmt.Sprintf("tag: %d", tag)
			item.Children = append(item.Children, tagSec)
			x := parser.u8()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 8,
				Name:       fmt.Sprintf("%v", math.Float64frombits(x)),
			})
			next += 8
			i++
		case 7:
			item.Name = fmt.Sprintf("[%d] class info", i+1)
			tagSec.Name = fmt.Sprintf("tag: %d", tag)
			item.Children = append(item.Children, tagSec)
			x := parser.u2()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("name index: %v", x),
			})
			next += 2
		case 8:
			item.Name = fmt.Sprintf("[%d] string constant", i+1)
			tagSec.Name = fmt.Sprintf("tag: %d", tag)
			item.Children = append(item.Children, tagSec)
			utf8Index := parser.u2()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("UTF-8 constant index: %v", utf8Index),
			})
			next += 2
		case 9:
			item.Name = fmt.Sprintf("[%d] field ref", i+1)
			tagSec.Name = fmt.Sprintf("tag: %d", tag)
			item.Children = append(item.Children, tagSec)
			classIndex := parser.u2()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("class info index: %v", classIndex),
			})
			next += 2
			nameAndTypeIndex := parser.u2()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("name and type index: %v", nameAndTypeIndex),
			})
			next += 2
		case 10:
			item.Name = fmt.Sprintf("[%d] method ref", i+1)
			tagSec.Name = fmt.Sprintf("tag: %d", tag)
			item.Children = append(item.Children, tagSec)
			classIndex := parser.u2()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("class info index: %v", classIndex),
			})
			next += 2
			nameAndTypeIndex := parser.u2()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("name and type index: %v", nameAndTypeIndex),
			})
			next += 2
		case 11:
			item.Name = fmt.Sprintf("[%d] interface method ref", i+1)
			tagSec.Name = fmt.Sprintf("tag: %d", tag)
			item.Children = append(item.Children, tagSec)
			classIndex := parser.u2()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("class info index: %v", classIndex),
			})
			next += 2
			nameAndTypeIndex := parser.u2()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("name and type index: %v", nameAndTypeIndex),
			})
			next += 2
		case 12:
			item.Name = fmt.Sprintf("[%d] name and type", i+1)
			tagSec.Name = fmt.Sprintf("tag: %d", tag)
			item.Children = append(item.Children, tagSec)
			nameIndex := parser.u2()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("name index: %v", nameIndex),
			})
			next += 2
			descriptorIndex := parser.u2()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("descriptor index: %v", descriptorIndex),
			})
			next += 2
		case 15:
			item.Name = fmt.Sprintf("[%d] method handle", i+1)
			tagSec.Name = fmt.Sprintf("tag: %d", tag)
			item.Children = append(item.Children, tagSec)
			referenceKind := parser.u1()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 1,
				Name:       fmt.Sprintf("reference kind: %v", referenceKind),
			})
			next += 1
			referenceIndex := parser.u2()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("reference index: %v", referenceIndex),
			})
			next += 2
		case 16:
			item.Name = fmt.Sprintf("[%d] method type", i+1)
			tagSec.Name = fmt.Sprintf("tag: %d", tag)
			item.Children = append(item.Children, tagSec)
			descriptorIndex := parser.u2()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("descriptor index: %v", descriptorIndex),
			})
			next += 2
		case 18:
			item.Name = fmt.Sprintf("[%d] invoke dynamic", i+1)
			tagSec.Name = fmt.Sprintf("tag: %d", tag)
			item.Children = append(item.Children, tagSec)
			bootstrapMethodIndex := parser.u2()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("bootstrap method attribute index: %v", bootstrapMethodIndex),
			})
			next += 2
			nameAndTypeIndex := parser.u2()
			item.Children = append(item.Children, Section{
				Id:         nextId(),
				StartIndex: next,
				EndIndex:   next + 2,
				Name:       fmt.Sprintf("name and type index: %v", nameAndTypeIndex),
			})
			next += 2
		default:
			log.Printf("What is a tag %d\n", tag)
			break loop
		}
		item.EndIndex = next
		children = append(children, item)
	}

	section = &Section{
		Id:         nextId(),
		StartIndex: index,
		EndIndex:   next,
		Name:       fmt.Sprintf("constant pool with %d items", len(children)-1),
		Children:   children,
	}
	return
}

var parsingFuncs = []func([]byte, int) (int, *Section){
	parseMagicNumber,
	parseVersion,
	parseConstantPool,
	parseAccessFlags,
	parseThisClass,
	parseSuperClass,
	parseInterfaces,
}

func parseClass(bytes []byte) []Section {
	globalId = 0
	index := 0
	var section *Section
	var sections []Section
	for _, f := range parsingFuncs {
		index, section = f(bytes, index)
		if section != nil {
			sections = append(sections, *section)
		}
	}
	return sections
}
