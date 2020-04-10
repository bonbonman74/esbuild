package bundler

import (
	"esbuild/fs"
	"esbuild/logging"
	"esbuild/parser"
	"esbuild/resolver"
	"path"
	"testing"
)

func expectBundledTS(t *testing.T, args bundled) {
	t.Run("", func(t *testing.T) {
		fs := fs.MockFS(args.files)
		resolver := resolver.NewResolver(fs, []string{".tsx", ".ts"})

		log, join := logging.NewDeferLog()
		bundle := ScanBundle(log, fs, resolver, args.entryPaths, args.parseOptions, args.bundleOptions)
		msgs := join()
		assertLog(t, msgs, args.expectedScanLog)

		// Stop now if there were any errors during the scan
		if hasErrors(msgs) {
			return
		}

		log, join = logging.NewDeferLog()
		args.bundleOptions.omitBootstrapForTests = true
		if args.bundleOptions.AbsOutputFile != "" {
			args.bundleOptions.AbsOutputDir = path.Dir(args.bundleOptions.AbsOutputFile)
		}
		results := bundle.Compile(log, args.bundleOptions)
		msgs = join()
		assertLog(t, msgs, args.expectedCompileLog)

		// Stop now if there were any errors during the compile
		if hasErrors(msgs) {
			return
		}

		assertEqual(t, len(results), len(args.expected))
		for _, result := range results {
			file := args.expected[result.JsAbsPath]
			path := "[" + result.JsAbsPath + "]\n"
			assertEqual(t, path+string(result.JsContents), path+file)
		}
	})
}

func TestTSDeclareConst(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare const require: any
				declare const exports: any;
				declare const module: any

				declare const foo: any
				let foo
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    let foo;
  }
}, 0);
`,
		},
	})
}

func TestTSDeclareLet(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare let require: any
				declare let exports: any;
				declare let module: any

				declare let foo: any
				let foo
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    let foo;
  }
}, 0);
`,
		},
	})
}

func TestTSDeclareVar(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare var require: any
				declare var exports: any;
				declare var module: any

				declare var foo: any
				let foo
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    let foo;
  }
}, 0);
`,
		},
	})
}

func TestTSDeclareClass(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare class require {}
				declare class exports {};
				declare class module {}

				declare class foo {}
				let foo
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    ;
    let foo;
  }
}, 0);
`,
		},
	})
}

func TestTSDeclareFunction(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare function require(): void
				declare function exports(): void;
				declare function module(): void

				declare function foo() {}
				let foo
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    let foo;
  }
}, 0);
`,
		},
	})
}

func TestTSDeclareNamespace(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare namespace require {}
				declare namespace exports {};
				declare namespace module {}

				declare namespace foo {}
				let foo
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    ;
    let foo;
  }
}, 0);
`,
		},
	})
}

func TestTSDeclareEnum(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare enum require {}
				declare enum exports {};
				declare enum module {}

				declare enum foo {}
				let foo
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    ;
    let foo;
  }
}, 0);
`,
		},
	})
}

func TestTSDeclareConstEnum(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				declare const enum require {}
				declare const enum exports {};
				declare const enum module {}

				declare const enum foo {}
				let foo
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    ;
    let foo;
  }
}, 0);
`,
		},
	})
}

func TestTSImportEmptyNamespace(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {ns} from './ns.ts'
				console.log(ns)
			`,
			"/ns.ts": `
				export namespace ns {}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expectedCompileLog: "/entry.ts: error: No matching export for import \"ns\"\n",
	})
}

// It's an error to import from a file that does not exist
func TestTSImportMissingFile(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {Something} from './doesNotExist.ts'
				let foo = new Something
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: "/entry.ts: error: Could not resolve \"./doesNotExist.ts\"\n",
	})
}

// It's not an error to import a type from a file that does not exist
func TestTSImportTypeOnlyFile(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				import {SomeType1} from './doesNotExist1.ts'
				import {SomeType2} from './doesNotExist2.ts'
				let foo: SomeType1
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.ts
    let foo;
  }
}, 0);
`,
		},
	})
}

func TestTSExportEquals(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.ts": `
				import b from './b.ts'
				console.log(b)
			`,
			"/b.ts": `
				export = [123, foo]
				function foo() {}
			`,
		},
		entryPaths: []string{"/a.ts"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			Bundle:        true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(require, exports, module) {
    // /b.ts
    function foo() {
    }
    module.exports = [123, foo];
  },

  0(require) {
    // /a.ts
    const b = require(1 /* ./b.ts */, true /* ES6 import */);
    console.log(b.default);
  }
}, 0);
`,
		},
	})
}

func TestTSMinifyEnum(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.ts": `
				enum Foo { A, B, C = Foo }
			`,
			"/b.ts": `
				export enum Foo { X, Y, Z = Foo }
			`,
		},
		entryPaths: []string{"/a.ts", "/b.ts"},
		parseOptions: parser.ParseOptions{
			MangleSyntax: true,
		},
		bundleOptions: BundleOptions{
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputDir:      "/",
		},
		expected: map[string]string{
			"/a.min.js": "var b;(function(a){a[a.A=0]=\"A\",a[a.B=1]=\"B\",a[a.C=a]=\"C\"})(b||(b={}));\n",
			"/b.min.js": "export var Foo;(function(a){a[a.X=0]=\"X\",a[a.Y=1]=\"Y\",a[a.Z=a]=\"Z\"})(Foo||(Foo={}));\n",
		},
	})
}

func TestTSMinifyNamespace(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.ts": `
				namespace Foo {
					export namespace Bar {
						foo(Foo, Bar)
					}
				}
			`,
			"/b.ts": `
				export namespace Foo {
					export namespace Bar {
						foo(Foo, Bar)
					}
				}
			`,
		},
		entryPaths: []string{"/a.ts", "/b.ts"},
		parseOptions: parser.ParseOptions{
			MangleSyntax: true,
		},
		bundleOptions: BundleOptions{
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputDir:      "/",
		},
		expected: map[string]string{
			"/a.min.js": "var b;(function(a){let c;(function(d){foo(a,d)})(c=a.Bar||(a.Bar={}))})(b||(b={}));\n",
			"/b.min.js": "export var Foo;(function(a){let b;(function(c){foo(a,c)})(b=a.Bar||(a.Bar={}))})(Foo||(Foo={}));\n",
		},
	})
}

func TestTSMinifyDerivedClass(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.ts": `
				class Foo extends Bar {
					foo = 1;
					bar = 2;
					constructor() {
						super();
						foo();
						bar();
					}
				}
			`,
		},
		entryPaths: []string{"/entry.ts"},
		parseOptions: parser.ParseOptions{
			MangleSyntax: true,
			Target:       parser.ES2015,
		},
		bundleOptions: BundleOptions{
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `class Foo extends Bar {
  constructor() {
    super();
    this.foo = 1;
    this.bar = 2;
    foo(), bar();
  }
}
`,
		},
	})
}
