//go:build ignore

// Generator for the .wasm fixtures in this directory.
//
//	go run gen.go
//
// Hand-crafts a few minimal WebAssembly modules by writing the raw
// binary format directly. Each module is small (<100 bytes) and
// exercises a different mix of sections so wasm2wat output is varied.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

const (
	secType     = 1
	secImport   = 2
	secFunction = 3
	secMemory   = 5
	secGlobal   = 6
	secExport   = 7
	secCode     = 10

	valI32 = 0x7f

	kindFunc   = 0x00
	kindMemory = 0x02
)

func main() {
	mustWrite("add.wasm", buildAdd())
	mustWrite("greet.wasm", buildGreet())
	mustWrite("counter.wasm", buildCounter())
	mustWrite("empty.wasm", header())
}

func mustWrite(name string, data []byte) {
	if err := os.WriteFile(name, data, 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", name, len(data))
}

// buildAdd emits a module exporting `add(i32, i32) -> i32` that
// returns its two arguments summed.
func buildAdd() []byte {
	var m bytes.Buffer
	m.Write(header())

	var types bytes.Buffer
	leb(&types, 1)
	types.Write(funcType([]byte{valI32, valI32}, []byte{valI32}))
	section(&m, secType, types.Bytes())

	var funcs bytes.Buffer
	leb(&funcs, 1)
	leb(&funcs, 0) // type index 0
	section(&m, secFunction, funcs.Bytes())

	var exports bytes.Buffer
	leb(&exports, 1)
	exportEntry(&exports, "add", kindFunc, 0)
	section(&m, secExport, exports.Bytes())

	body := []byte{
		0x00,             // 0 local groups
		0x20, 0x00,       // local.get 0
		0x20, 0x01,       // local.get 1
		0x6a,             // i32.add
		0x0b,             // end
	}
	section(&m, secCode, codeSection([][]byte{body}))

	return m.Bytes()
}

// buildGreet emits a module that imports env.greet and exports a main
// function that calls it.
func buildGreet() []byte {
	var m bytes.Buffer
	m.Write(header())

	var types bytes.Buffer
	leb(&types, 1)
	types.Write(funcType(nil, nil))
	section(&m, secType, types.Bytes())

	var imports bytes.Buffer
	leb(&imports, 1)
	importEntry(&imports, "env", "greet", kindFunc, 0)
	section(&m, secImport, imports.Bytes())

	var funcs bytes.Buffer
	leb(&funcs, 1)
	leb(&funcs, 0)
	section(&m, secFunction, funcs.Bytes())

	var exports bytes.Buffer
	leb(&exports, 1)
	// Imported func is index 0; locally-defined main is index 1.
	exportEntry(&exports, "main", kindFunc, 1)
	section(&m, secExport, exports.Bytes())

	body := []byte{
		0x00,       // 0 locals
		0x10, 0x00, // call $greet (func index 0)
		0x0b,       // end
	}
	section(&m, secCode, codeSection([][]byte{body}))

	return m.Bytes()
}

// buildCounter emits a module with a mutable global, a memory, and an
// inc() -> i32 function that increments and returns the global. Memory
// is exported alongside inc to exercise multi-export rendering.
func buildCounter() []byte {
	var m bytes.Buffer
	m.Write(header())

	var types bytes.Buffer
	leb(&types, 1)
	types.Write(funcType(nil, []byte{valI32}))
	section(&m, secType, types.Bytes())

	var funcs bytes.Buffer
	leb(&funcs, 1)
	leb(&funcs, 0)
	section(&m, secFunction, funcs.Bytes())

	var memories bytes.Buffer
	leb(&memories, 1)
	memories.WriteByte(0x00) // limits flag: only min present
	leb(&memories, 1)        // min = 1 page
	section(&m, secMemory, memories.Bytes())

	var globals bytes.Buffer
	leb(&globals, 1)
	globals.WriteByte(valI32) // value type
	globals.WriteByte(0x01)   // mutable
	globals.WriteByte(0x41)   // i32.const
	leb(&globals, 0)          // 0
	globals.WriteByte(0x0b)   // end
	section(&m, secGlobal, globals.Bytes())

	var exports bytes.Buffer
	leb(&exports, 2)
	exportEntry(&exports, "memory", kindMemory, 0)
	exportEntry(&exports, "inc", kindFunc, 0)
	section(&m, secExport, exports.Bytes())

	body := []byte{
		0x00,             // 0 locals
		0x23, 0x00,       // global.get 0
		0x41, 0x01,       // i32.const 1
		0x6a,             // i32.add
		0x24, 0x00,       // global.set 0
		0x23, 0x00,       // global.get 0
		0x0b,             // end
	}
	section(&m, secCode, codeSection([][]byte{body}))

	return m.Bytes()
}

// header is the 8-byte wasm preamble: magic + version 1.
func header() []byte {
	b := make([]byte, 8)
	copy(b, "\x00asm")
	binary.LittleEndian.PutUint32(b[4:], 1)
	return b
}

// section writes a one-byte section id, the LEB128-encoded payload
// length, and the payload itself.
func section(w *bytes.Buffer, id byte, payload []byte) {
	w.WriteByte(id)
	leb(w, uint32(len(payload)))
	w.Write(payload)
}

// funcType encodes a wasm functype: 0x60 followed by the parameter and
// result vectors.
func funcType(params, results []byte) []byte {
	var b bytes.Buffer
	b.WriteByte(0x60)
	leb(&b, uint32(len(params)))
	b.Write(params)
	leb(&b, uint32(len(results)))
	b.Write(results)
	return b.Bytes()
}

// codeSection wraps each function body with its LEB128 length prefix
// and prepends the count.
func codeSection(bodies [][]byte) []byte {
	var b bytes.Buffer
	leb(&b, uint32(len(bodies)))
	for _, body := range bodies {
		leb(&b, uint32(len(body)))
		b.Write(body)
	}
	return b.Bytes()
}

// exportEntry writes name_len, name, kind, index for a single export.
func exportEntry(w *bytes.Buffer, name string, kind byte, index uint32) {
	leb(w, uint32(len(name)))
	w.WriteString(name)
	w.WriteByte(kind)
	leb(w, index)
}

// importEntry writes module_len, module, name_len, name, kind, type
// index for a single import.
func importEntry(w *bytes.Buffer, module, name string, kind byte, typeIndex uint32) {
	leb(w, uint32(len(module)))
	w.WriteString(module)
	leb(w, uint32(len(name)))
	w.WriteString(name)
	w.WriteByte(kind)
	leb(w, typeIndex)
}

// leb writes an unsigned LEB128.
func leb(w *bytes.Buffer, v uint32) {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		w.WriteByte(b)
		if v == 0 {
			return
		}
	}
}
