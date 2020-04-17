package bundler

import (
	"esbuild/fs"
	"esbuild/logging"
	"esbuild/parser"
	"esbuild/resolver"
	"path"
	"testing"
)

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func assertLog(t *testing.T, msgs []logging.Msg, expected string) {
	text := ""
	for _, msg := range msgs {
		text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
	}
	assertEqual(t, text, expected)
}

func hasErrors(msgs []logging.Msg) bool {
	for _, msg := range msgs {
		if msg.Kind == logging.Error {
			return true
		}
	}
	return false
}

type bundled struct {
	files              map[string]string
	entryPaths         []string
	expected           map[string]string
	expectedScanLog    string
	expectedCompileLog string
	parseOptions       parser.ParseOptions
	bundleOptions      BundleOptions
	resolveOptions     resolver.ResolveOptions
}

func expectBundled(t *testing.T, args bundled) {
	t.Run("", func(t *testing.T) {
		fs := fs.MockFS(args.files)
		args.resolveOptions.ExtensionOrder = []string{".tsx", ".ts", ".jsx", ".js", ".json"}
		resolver := resolver.NewResolver(fs, args.resolveOptions)

		log, join := logging.NewDeferLog()
		bundle := ScanBundle(log, fs, resolver, args.entryPaths, args.parseOptions, args.bundleOptions)
		msgs := join()
		assertLog(t, msgs, args.expectedScanLog)

		// Stop now if there were any errors during the scan
		if hasErrors(msgs) {
			return
		}

		log, join = logging.NewDeferLog()
		args.bundleOptions.omitRuntimeForTests = true
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

func TestSimpleES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {fn} from './foo'
				console.log(fn())
			`,
			"/foo.js": `
				export function fn() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			ModuleName:    "testModule",
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `let testModule = bootstrap({
  0() {
    // /foo.js
    function fn() {
      return 123;
    }

    // /entry.js
    console.log(fn());
  }
}, 0);
`,
		},
	})
}

func TestSimpleCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const fn = require('./foo')
				console.log(fn())
			`,
			"/foo.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			ModuleName:    "testModule",
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `let testModule = bootstrap({
  1(exports, module) {
    // /foo.js
    module.exports = function() {
      return 123;
    };
  },

  0() {
    // /entry.js
    const fn = __require(1 /* ./foo */);
    console.log(fn());
  }
}, 0);
`,
		},
	})
}

// This test makes sure that require() calls are still recognized in nested
// scopes. It guards against bugs where require() calls are only recognized in
// the top-level module scope.
func TestNestedCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function nestedScope() {
					const fn = require('./foo')
					console.log(fn())
				}
				nestedScope()
			`,
			"/foo.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports, module) {
    // /foo.js
    module.exports = function() {
      return 123;
    };
  },

  0() {
    // /entry.js
    function nestedScope() {
      const fn = __require(1 /* ./foo */);
      console.log(fn());
    }
    nestedScope();
  }
}, 0);
`,
		},
	})
}

func TestCommonJSFromES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const fn = require('./foo')
				console.log(fn())
			`,
			"/foo.js": `
				export function fn() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports) {
    // /foo.js
    __export(exports, {
      fn: () => fn
    });
    function fn() {
      return 123;
    }
  },

  0() {
    // /entry.js
    const fn = __require(1 /* ./foo */);
    console.log(fn());
  }
}, 0);
`,
		},
	})
}

func TestES6FromCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {fn} from './foo'
				console.log(fn())
			`,
			"/foo.js": `
				exports.fn = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports) {
    // /foo.js
    exports.fn = function() {
      return 123;
    };
  },

  0() {
    // /entry.js
    const foo = __import(1 /* ./foo */);
    console.log(foo.fn());
  }
}, 0);
`,
		},
	})
}

// This test makes sure that ES6 imports are still recognized in nested
// scopes. It guards against bugs where require() calls are only recognized in
// the top-level module scope.
func TestNestedES6FromCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {fn} from './foo'
				(() => {
					console.log(fn())
				})()
			`,
			"/foo.js": `
				exports.fn = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports) {
    // /foo.js
    exports.fn = function() {
      return 123;
    };
  },

  0() {
    // /entry.js
    const foo = __import(1 /* ./foo */);
    (() => {
      console.log(foo.fn());
    })();
  }
}, 0);
`,
		},
	})
}

func TestExportForms(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export default 123
				export var v = 234
				export let l = 234
				export const c = 234
				export {Class as C}
				export function Fn() {}
				export class Class {}
				export * from './a'
				export * as b from './b'
			`,
			"/a.js": "export const abc = undefined",
			"/b.js": "export const xyz = null",
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  2(exports) {
    // /a.js
    const abc = void 0;

    // /b.js
    var b = {};
    __export(b, {
      xyz: () => xyz
    });
    const xyz = null;

    // /entry.js
    __export(exports, {
      C: () => Class,
      Class: () => Class,
      Fn: () => Fn,
      abc: () => abc,
      b: () => b,
      c: () => c,
      default: () => default2,
      l: () => l,
      v: () => v
    });
    const default2 = 123;
    var v = 234;
    let l = 234;
    const c = 234;
    function Fn() {
    }
    class Class {
    }
  }
}, 2);
`,
		},
	})
}

func TestExportFormsWithMinifyIdentifiersAndNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				export default 123
				export var varName = 234
				export let letName = 234
				export const constName = 234
				function Func2() {}
				class Class2 {}
				export {Class as Cls, Func2 as Fn2, Class2 as Cls2}
				export function Func() {}
				export class Class {}
				export * from './a'
				export * as fromB from './b'
			`,
			"/b.js": "export default function() {}",
			"/c.js": "export default function foo() {}",
			"/d.js": "export default class {}",
			"/e.js": "export default class Foo {}",
		},
		entryPaths: []string{
			"/a.js",
			"/b.js",
			"/c.js",
			"/d.js",
			"/e.js",
		},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputDir:      "/out",
		},
		expected: map[string]string{
			"/out/a.js": `export default 123;
export var varName = 234;
export let letName = 234;
export const constName = 234;
function a() {
}
class b {
}
export {Class as Cls, a as Fn2, b as Cls2};
export function Func() {
}
export class Class {
}
export * from "./a";
export * as fromB from "./b";
`,
			"/out/b.js": `export default function() {
}
`,
			"/out/c.js": `export default function b() {
}
`,
			"/out/d.js": `export default class {
}
`,
			"/out/e.js": `export default class b {
}
`,
		},
	})
}

func TestImportFormsWithNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'foo'
				import {} from 'foo'
				import * as ns from 'foo'
				import {a, b as c} from 'foo'
				import def from 'foo'
				import def2, * as ns2 from 'foo'
				import def3, {a2, b as c3} from 'foo'
				const imp = [
					import('foo'),
					function nested() { return import('foo') },
				]
				console.log(ns, a, c, def, def2, ns2, def3, a2, c3, imp)
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `import "foo";
import {} from "foo";
import * as ns from "foo";
import {a, b as c} from "foo";
import def from "foo";
import def2, * as ns2 from "foo";
import def3, {a2, b as c3} from "foo";
const imp = [import("foo"), function nested() {
  return import("foo");
}];
console.log(ns, a, c, def, def2, ns2, def3, a2, c3, imp);
`,
		},
	})
}

func TestImportFormsWithMinifyIdentifiersAndNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'foo'
				import {} from 'foo'
				import * as ns from 'foo'
				import {a, b as c} from 'foo'
				import def from 'foo'
				import def2, * as ns2 from 'foo'
				import def3, {a2, b as c3} from 'foo'
				const imp = [
					import('foo'),
					function nested() { return import('foo') },
				]
				console.log(ns, a, c, def, def2, ns2, def3, a2, c3, imp)
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:        false,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
		expected: map[string]string{
			"/out.js": `import "foo";
import {} from "foo";
import * as a from "foo";
import {a as b, b as c} from "foo";
import d from "foo";
import f, * as e from "foo";
import g, {a2 as h, b as i} from "foo";
const j = [import("foo"), function p() {
  return import("foo");
}];
console.log(a, b, c, d, f, e, g, h, i, j);
`,
		},
	})
}

func TestExportFormsCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				require('./commonjs')
				require('./c')
				require('./d')
				require('./e')
				require('./f')
				require('./g')
				require('./h')
			`,
			"/commonjs.js": `
				export default 123
				export var v = 234
				export let l = 234
				export const c = 234
				export {Class as C}
				export function Fn() {}
				export class Class {}
				export * from './a'
				export * as b from './b'
			`,
			"/a.js": "export const abc = undefined",
			"/b.js": "export const xyz = null",
			"/c.js": "export default class {}",
			"/d.js": "export default class Foo {}",
			"/e.js": "export default function() {}",
			"/f.js": "export default function foo() {}",
			"/g.js": "export default async function() {}",
			"/h.js": "export default async function foo() {}",
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0(exports) {
    // /a.js
    __export(exports, {
      abc: () => abc
    });
    const abc = void 0;
  },

  1(exports) {
    // /b.js
    __export(exports, {
      xyz: () => xyz
    });
    const xyz = null;
  },

  3(exports) {
    // /commonjs.js
    __export(exports, {
      C: () => Class,
      Class: () => Class,
      Fn: () => Fn,
      b: () => b,
      c: () => c,
      default: () => default2,
      l: () => l,
      v: () => v
    });
    const b = __import(1 /* ./b */);
    const default2 = 123;
    var v = 234;
    let l = 234;
    const c = 234;
    function Fn() {
    }
    class Class {
    }
  },

  2(exports) {
    // /c.js
    __export(exports, {
      default: () => default2
    });
    class default2 {
    }
  },

  4(exports) {
    // /d.js
    __export(exports, {
      default: () => Foo
    });
    class Foo {
    }
  },

  5(exports) {
    // /e.js
    __export(exports, {
      default: () => default2
    });
    function default2() {
    }
  },

  7(exports) {
    // /f.js
    __export(exports, {
      default: () => foo
    });
    function foo() {
    }
  },

  8(exports) {
    // /g.js
    __export(exports, {
      default: () => default2
    });
    async function default2() {
    }
  },

  9(exports) {
    // /h.js
    __export(exports, {
      default: () => foo
    });
    async function foo() {
    }
  },

  6() {
    // /entry.js
    __require(3 /* ./commonjs */);
    __require(2 /* ./c */);
    __require(4 /* ./d */);
    __require(5 /* ./e */);
    __require(7 /* ./f */);
    __require(8 /* ./g */);
    __require(9 /* ./h */);
  }
}, 6);
`,
		},
	})
}

func TestExportSelf(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export const foo = 123
				export * from './entry'
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0(exports) {
    // /entry.js
    __export(exports, {
      foo: () => foo
    });
    const foo = 123;
  }
}, 0);
`,
		},
	})
}

func TestExportSelfAsNamespace(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export const foo = 123
				export * as ns from './entry'
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0(exports) {
    // /entry.js
    __export(exports, {
      foo: () => foo,
      ns: () => exports
    });
    const foo = 123;
  }
}, 0);
`,
		},
	})
}

func TestJSXImportsCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				import {elem, frag} from './custom-react'
				console.log(<div/>, <>fragment</>)
			`,
			"/custom-react.js": `
				module.exports = {}
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			JSX: parser.JSXOptions{
				Factory:  []string{"elem"},
				Fragment: []string{"frag"},
			},
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0(exports, module) {
    // /custom-react.js
    module.exports = {};
  },

  1() {
    // /entry.jsx
    const custom_react = __import(0 /* ./custom-react */);
    console.log(custom_react.elem("div", null), custom_react.elem(custom_react.frag, null, "fragment"));
  }
}, 1);
`,
		},
	})
}

func TestJSXImportsES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.jsx": `
				import {elem, frag} from './custom-react'
				console.log(<div/>, <>fragment</>)
			`,
			"/custom-react.js": `
				export function elem() {}
				export function frag() {}
			`,
		},
		entryPaths: []string{"/entry.jsx"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
			JSX: parser.JSXOptions{
				Factory:  []string{"elem"},
				Fragment: []string{"frag"},
			},
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1() {
    // /custom-react.js
    function elem() {
    }
    function frag() {
    }

    // /entry.jsx
    console.log(elem("div", null), elem(frag, null, "fragment"));
  }
}, 1);
`,
		},
	})
}

func TestJSXSyntaxInJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(<div/>)
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: Unexpected "<"
`,
	})
}

func TestJSXSyntaxInJSWithJSXLoader(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(<div/>)
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]Loader{
				".js": LoaderJSX,
			},
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.js
    console.log(React.createElement("div", null));
  }
}, 0);
`,
		},
	})
}

func TestNodeModules(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  0(exports, module) {
    // /Users/user/project/node_modules/demo-pkg/index.js
    module.exports = function() {
      return 123;
    };
  },

  1() {
    // /Users/user/project/src/entry.js
    const demo_pkg = __import(0 /* demo-pkg */);
    console.log(demo_pkg.default());
  }
}, 1);
`,
		},
	})
}

func TestPackageJsonMain(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./custom-main.js"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/custom-main.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  0(exports, module) {
    // /Users/user/project/node_modules/demo-pkg/custom-main.js
    module.exports = function() {
      return 123;
    };
  },

  1() {
    // /Users/user/project/src/entry.js
    const demo_pkg = __import(0 /* demo-pkg */);
    console.log(demo_pkg.default());
  }
}, 1);
`,
		},
	})
}

func TestTsconfigJsonBaseUrl(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/app/entry.js": `
				import fn from 'lib/util'
				console.log(fn())
			`,
			"/Users/user/project/src/tsconfig.json": `
				{
					"compilerOptions": {
						"baseUrl": "."
					}
				}
			`,
			"/Users/user/project/src/lib/util.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/app/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1(exports, module) {
    // /Users/user/project/src/lib/util.js
    module.exports = function() {
      return 123;
    };
  },

  0() {
    // /Users/user/project/src/app/entry.js
    const util = __import(1 /* lib/util */);
    console.log(util.default());
  }
}, 0);
`,
		},
	})
}

func TestPackageJsonBrowserString(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": "./browser"
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/browser.js": `
				module.exports = function() {
					return 123
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  0(exports, module) {
    // /Users/user/project/node_modules/demo-pkg/browser.js
    module.exports = function() {
      return 123;
    };
  },

  1() {
    // /Users/user/project/src/entry.js
    const demo_pkg = __import(0 /* demo-pkg */);
    console.log(demo_pkg.default());
  }
}, 1);
`,
		},
	})
}

func TestPackageJsonBrowserMapRelativeToRelative(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main",
					"browser": {
						"./main.js": "./main-browser",
						"./lib/util.js": "./lib/util-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				const util = require('./lib/util')
				module.exports = function() {
					return ['main', util]
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main-browser.js": `
				const util = require('./lib/util')
				module.exports = function() {
					return ['main-browser', util]
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/lib/util.js": `
				module.exports = 'util'
			`,
			"/Users/user/project/node_modules/demo-pkg/lib/util-browser.js": `
				module.exports = 'util-browser'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  0(exports, module) {
    // /Users/user/project/node_modules/demo-pkg/lib/util-browser.js
    module.exports = "util-browser";
  },

  1(exports, module) {
    // /Users/user/project/node_modules/demo-pkg/main-browser.js
    const util = __require(0 /* ./lib/util */);
    module.exports = function() {
      return ["main-browser", util];
    };
  },

  2() {
    // /Users/user/project/src/entry.js
    const demo_pkg = __import(1 /* demo-pkg */);
    console.log(demo_pkg.default());
  }
}, 2);
`,
		},
	})
}

func TestPackageJsonBrowserMapRelativeToModule(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main",
					"browser": {
						"./util.js": "util-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				const util = require('./util')
				module.exports = function() {
					return ['main', util]
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/util.js": `
				module.exports = 'util'
			`,
			"/Users/user/project/node_modules/util-browser/index.js": `
				module.exports = 'util-browser'
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1(exports, module) {
    // /Users/user/project/node_modules/util-browser/index.js
    module.exports = "util-browser";
  },

  0(exports, module) {
    // /Users/user/project/node_modules/demo-pkg/main.js
    const util = __require(1 /* ./util */);
    module.exports = function() {
      return ["main", util];
    };
  },

  2() {
    // /Users/user/project/src/entry.js
    const demo_pkg = __import(0 /* demo-pkg */);
    console.log(demo_pkg.default());
  }
}, 2);
`,
		},
	})
}

func TestPackageJsonBrowserMapRelativeDisabled(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"main": "./main",
					"browser": {
						"./util-node.js": false
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/main.js": `
				const util = require('./util-node')
				module.exports = function(obj) {
					return util.inspect(obj)
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/util-node.js": `
				module.exports = require('util')
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1() {
    // /Users/user/project/node_modules/demo-pkg/util-node.js
  },

  0(exports, module) {
    // /Users/user/project/node_modules/demo-pkg/main.js
    const util = __require(1 /* ./util-node */);
    module.exports = function(obj) {
      return util.inspect(obj);
    };
  },

  2() {
    // /Users/user/project/src/entry.js
    const demo_pkg = __import(0 /* demo-pkg */);
    console.log(demo_pkg.default());
  }
}, 2);
`,
		},
	})
}

func TestPackageJsonBrowserMapModuleToRelative(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"node-pkg": "./node-pkg-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/node-pkg-browser.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const fn = require('node-pkg')
				module.exports = function() {
					return fn()
				}
			`,
			"/Users/user/project/node_modules/node-pkg/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1(exports, module) {
    // /Users/user/project/node_modules/demo-pkg/node-pkg-browser.js
    module.exports = function() {
      return 123;
    };
  },

  0(exports, module) {
    // /Users/user/project/node_modules/demo-pkg/index.js
    const fn = __require(1 /* node-pkg */);
    module.exports = function() {
      return fn();
    };
  },

  2() {
    // /Users/user/project/src/entry.js
    const demo_pkg = __import(0 /* demo-pkg */);
    console.log(demo_pkg.default());
  }
}, 2);
`,
		},
	})
}

func TestPackageJsonBrowserMapModuleToModule(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"node-pkg": "node-pkg-browser"
					}
				}
			`,
			"/Users/user/project/node_modules/node-pkg-browser/index.js": `
				module.exports = function() {
					return 123
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const fn = require('node-pkg')
				module.exports = function() {
					return fn()
				}
			`,
			"/Users/user/project/node_modules/node-pkg/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1(exports, module) {
    // /Users/user/project/node_modules/node-pkg-browser/index.js
    module.exports = function() {
      return 123;
    };
  },

  0(exports, module) {
    // /Users/user/project/node_modules/demo-pkg/index.js
    const fn = __require(1 /* node-pkg */);
    module.exports = function() {
      return fn();
    };
  },

  2() {
    // /Users/user/project/src/entry.js
    const demo_pkg = __import(0 /* demo-pkg */);
    console.log(demo_pkg.default());
  }
}, 2);
`,
		},
	})
}

func TestPackageJsonBrowserMapModuleDisabled(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import fn from 'demo-pkg'
				console.log(fn())
			`,
			"/Users/user/project/node_modules/demo-pkg/package.json": `
				{
					"browser": {
						"node-pkg": false
					}
				}
			`,
			"/Users/user/project/node_modules/demo-pkg/index.js": `
				const fn = require('node-pkg')
				module.exports = function() {
					return fn()
				}
			`,
			"/Users/user/project/node_modules/node-pkg/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1() {
    // /Users/user/project/node_modules/node-pkg/index.js
  },

  0(exports, module) {
    // /Users/user/project/node_modules/demo-pkg/index.js
    const fn = __require(1 /* node-pkg */);
    module.exports = function() {
      return fn();
    };
  },

  2() {
    // /Users/user/project/src/entry.js
    const demo_pkg = __import(0 /* demo-pkg */);
    console.log(demo_pkg.default());
  }
}, 2);
`,
		},
	})
}

func TestPackageJsonBrowserMapAvoidMissing(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import 'component-classes'
			`,
			"/Users/user/project/node_modules/component-classes/package.json": `
				{
					"browser": {
						"indexof": "component-indexof"
					}
				}
			`,
			"/Users/user/project/node_modules/component-classes/index.js": `
				try {
					var index = require('indexof');
				} catch (err) {
					var index = require('component-indexof');
				}
			`,
			"/Users/user/project/node_modules/component-indexof/index.js": `
				module.exports = function() {
					return 234
				}
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1(exports, module) {
    // /Users/user/project/node_modules/component-indexof/index.js
    module.exports = function() {
      return 234;
    };
  },

  2() {
    // /Users/user/project/node_modules/component-classes/index.js
    try {
      var index2 = __require(1 /* indexof */);
    } catch (err) {
      var index2 = __require(1 /* component-indexof */);
    }

    // /Users/user/project/src/entry.js
  }
}, 2);
`,
		},
	})
}

func TestRequireChildDirCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				console.log(require('./dir'))
			`,
			"/Users/user/project/src/dir/index.js": `
				module.exports = 123
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0(exports, module) {
    // /Users/user/project/src/dir/index.js
    module.exports = 123;
  },

  1() {
    // /Users/user/project/src/entry.js
    console.log(__require(0 /* ./dir */));
  }
}, 1);
`,
		},
	})
}

func TestRequireChildDirES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import value from './dir'
				console.log(value)
			`,
			"/Users/user/project/src/dir/index.js": `
				export default 123
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1() {
    // /Users/user/project/src/dir/index.js
    const default2 = 123;

    // /Users/user/project/src/entry.js
    console.log(default2);
  }
}, 1);
`,
		},
	})
}

func TestRequireParentDirCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/dir/entry.js": `
				console.log(require('..'))
			`,
			"/Users/user/project/src/index.js": `
				module.exports = 123
			`,
		},
		entryPaths: []string{"/Users/user/project/src/dir/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports, module) {
    // /Users/user/project/src/index.js
    module.exports = 123;
  },

  0() {
    // /Users/user/project/src/dir/entry.js
    console.log(__require(1 /* .. */));
  }
}, 0);
`,
		},
	})
}

func TestRequireParentDirES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/dir/entry.js": `
				import value from '..'
				console.log(value)
			`,
			"/Users/user/project/src/index.js": `
				export default 123
			`,
		},
		entryPaths: []string{"/Users/user/project/src/dir/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /Users/user/project/src/index.js
    const default2 = 123;

    // /Users/user/project/src/dir/entry.js
    console.log(default2);
  }
}, 0);
`,
		},
	})
}

func TestPackageImportMissingES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import fn, {x as a, y as b} from './foo'
				console.log(fn(a, b))
			`,
			"/foo.js": `
				export const x = 132
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedCompileLog: `/entry.js: error: No matching export for import "default"
/entry.js: error: No matching export for import "y"
`,
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /foo.js
    const x = 132;

    // /entry.js
    console.log(fn(x, b));
  }
}, 0);
`,
		},
	})
}

func TestPackageImportMissingCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import fn, {x as a, y as b} from './foo'
				console.log(fn(a, b))
			`,
			"/foo.js": `
				exports.x = 132
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports) {
    // /foo.js
    exports.x = 132;
  },

  0() {
    // /entry.js
    const foo = __import(1 /* ./foo */);
    console.log(foo.default(foo.x, foo.y));
  }
}, 0);
`,
		},
	})
}

func TestDotImport(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {x} from '.'
				console.log(x)
			`,
			"/index.js": `
				exports.x = 123
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports) {
    // /index.js
    exports.x = 123;
  },

  0() {
    // /entry.js
    const _ = __import(1 /* . */);
    console.log(_.x);
  }
}, 0);
`,
		},
	})
}

func TestRequireWithTemplate(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				console.log(require('./b'))
				console.log(require(` + "`./b`" + `))
			`,
			"/b.js": `
				exports.x = 123
			`,
		},
		entryPaths: []string{"/a.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports) {
    // /b.js
    exports.x = 123;
  },

  0() {
    // /a.js
    console.log(__require(1 /* ./b */));
    console.log(__require(1 /* ./b */));
  }
}, 0);
`,
		},
	})
}

func TestDynamicImportWithTemplate(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/a.js": `
				import('./b').then(ns => console.log(ns))
				import(` + "`./b`" + `).then(ns => console.log(ns))
			`,
			"/b.js": `
				exports.x = 123
			`,
		},
		entryPaths: []string{"/a.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports) {
    // /b.js
    exports.x = 123;
  },

  0() {
    // /a.js
    Promise.resolve().then(() => __import(1 /* ./b */)).then((ns) => console.log(ns));
    Promise.resolve().then(() => __import(1 /* ./b */)).then((ns) => console.log(ns));
  }
}, 0);
`,
		},
	})
}

func TestRequireAndDynamicImportInvalidTemplate(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				require(tag` + "`./b`" + `)
				require(` + "`./${b}`" + `)
				import(tag` + "`./b`" + `)
				import(` + "`./${b}`" + `)
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: The argument to require() must be a string literal
/entry.js: error: The argument to require() must be a string literal
/entry.js: error: The argument to import() must be a string literal
/entry.js: error: The argument to import() must be a string literal
`,
	})
}

func TestRequireJson(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.json'))
			`,
			"/test.json": `
				{
					"a": true,
					"b": 123,
					"c": [null]
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports, module) {
    // /test.json
    module.exports = {
      a: true,
      b: 123,
      c: [null]
    };
  },

  0() {
    // /entry.js
    console.log(__require(1 /* ./test.json */));
  }
}, 0);
`,
		},
	})
}

func TestRequireTxt(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.txt'))
			`,
			"/test.txt": `This is a test.`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports, module) {
    // /test.txt
    module.exports = "This is a test.";
  },

  0() {
    // /entry.js
    console.log(__require(1 /* ./test.txt */));
  }
}, 0);
`,
		},
	})
}

func TestRequireCustomExtensionString(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.custom'))
			`,
			"/test.custom": `This is a test.`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]Loader{
				".js":     LoaderJS,
				".custom": LoaderText,
			},
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports, module) {
    // /test.custom
    module.exports = "This is a test.";
  },

  0() {
    // /entry.js
    console.log(__require(1 /* ./test.custom */));
  }
}, 0);
`,
		},
	})
}

func TestRequireCustomExtensionBase64(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test.custom'))
			`,
			"/test.custom": "a\x00b\x80c\xFFd",
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
			ExtensionToLoader: map[string]Loader{
				".js":     LoaderJS,
				".custom": LoaderBase64,
			},
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports, module) {
    // /test.custom
    module.exports = "YQBigGP/ZA==";
  },

  0() {
    // /entry.js
    console.log(__require(1 /* ./test.custom */));
  }
}, 0);
`,
		},
	})
}

func TestRequireBadExtension(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('./test'))
			`,
			"/test": `This is a test.`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: File extension not supported: /test
`,
	})
}

func TestFalseRequire(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				(require => require('/test.txt'))()
			`,
			"/test.txt": `This is a test.`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.js
    ((require3) => require3("/test.txt"))();
  }
}, 0);
`,
		},
	})
}

func TestRequireWithoutCall(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const req = require
				req('./entry')
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: "require" must not be called indirectly
`,
	})
}

func TestNestedRequireWithoutCall(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				(() => {
					const req = require
					req('./entry')
				})()
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expectedScanLog: `/entry.js: error: "require" must not be called indirectly
`,
	})
}

// Test a workaround for the "moment" library
func TestRequireWithoutCallInsideTry(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				try {
					oldLocale = globalLocale._abbr;
					var aliasedRequire = require;
					aliasedRequire('./locale/' + name);
					getSetGlobalLocale(oldLocale);
				} catch (e) {}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.js
    try {
      oldLocale = globalLocale._abbr;
      var aliasedRequire = null;
      aliasedRequire("./locale/" + name);
      getSetGlobalLocale(oldLocale);
    } catch (e) {
    }
  }
}, 0);
`,
		},
	})
}

func TestSourceMap(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/Users/user/project/src/entry.js": `
				import {bar} from './bar'
				function foo() { bar() }
				foo()
			`,
			"/Users/user/project/src/bar.js": `
				export function bar() { throw new Error('test') }
			`,
		},
		entryPaths: []string{"/Users/user/project/src/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			SourceMap:     true,
			AbsOutputFile: "/Users/user/project/out.js",
		},
		expected: map[string]string{
			"/Users/user/project/out.js": `bootstrap({
  1() {
    // /Users/user/project/src/bar.js
    function bar2() {
      throw new Error("test");
    }

    // /Users/user/project/src/entry.js
    function foo() {
      bar2();
    }
    foo();
  }
}, 1);
//# sourceMappingURL=out.js.map
`,
		},
	})
}

// This test covers a bug where a "var" in a nested scope did not correctly
// bind with references to that symbol in sibling scopes. Instead, the
// references were incorrectly considered to be unbound even though the symbol
// should be hoisted. This caused the renamer to name them different things to
// avoid a collision, which changed the meaning of the code.
func TestNestedScopeBug(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				(() => {
					function a() {
						b()
					}
					{
						var b = () => {}
					}
					a()
				})()
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.js
    (() => {
      function a() {
        b();
      }
      {
        var b = () => {
        };
      }
      a();
    })();
  }
}, 0);
`,
		},
	})
}

func TestHashbangBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `#!/usr/bin/env node
				process.exit(0);
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.js
    process.exit(0);
  }
}, 0);
`,
		},
	})
}

func TestHashbangNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `#!/usr/bin/env node
				process.exit(0);
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `#!/usr/bin/env node
process.exit(0);
`,
		},
	})
}

func TestTypeofRequireBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(typeof require);
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.js
    console.log("function");
  }
}, 0);
`,
		},
	})
}

func TestTypeofRequireNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(typeof require);
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `console.log(typeof require);
`,
		},
	})
}

func TestRequireFSBrowser(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('fs'))
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		resolveOptions: resolver.ResolveOptions{
			Platform: resolver.PlatformBrowser,
		},
		expectedScanLog: "/entry.js: error: Could not resolve \"fs\"\n",
	})
}

func TestRequireFSNode(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(require('fs'))
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		resolveOptions: resolver.ResolveOptions{
			Platform: resolver.PlatformNode,
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.js
    console.log(require("fs"));
  }
}, 0);
`,
		},
	})
}

func TestImportFSBrowser(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'fs'
				import * as fs from 'fs'
				import defaultValue from 'fs'
				import {readFileSync} from 'fs'
				console.log(fs, readFileSync, defaultValue)
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		resolveOptions: resolver.ResolveOptions{
			Platform: resolver.PlatformBrowser,
		},
		expectedScanLog: `/entry.js: error: Could not resolve "fs"
/entry.js: error: Could not resolve "fs"
/entry.js: error: Could not resolve "fs"
/entry.js: error: Could not resolve "fs"
`,
	})
}

func TestImportFSNode(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import 'fs'
				import * as fs from 'fs'
				import defaultValue from 'fs'
				import {readFileSync} from 'fs'
				console.log(fs, readFileSync, defaultValue)
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		resolveOptions: resolver.ResolveOptions{
			Platform: resolver.PlatformNode,
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.js
    const fs = require("fs");
    const fs2 = require("fs");
    const fs3 = require("fs");
    const fs4 = require("fs");
    console.log(fs2, fs4.readFileSync, fs3.default);
  }
}, 0);
`,
		},
	})
}

func TestExportFSBrowser(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * as fs from 'fs'
				export {readFileSync} from 'fs'
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		resolveOptions: resolver.ResolveOptions{
			Platform: resolver.PlatformBrowser,
		},
		expectedScanLog: `/entry.js: error: Could not resolve "fs"
/entry.js: error: Could not resolve "fs"
`,
	})
}

func TestExportFSNode(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * as fs from 'fs'
				export {readFileSync} from 'fs'
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		resolveOptions: resolver.ResolveOptions{
			Platform: resolver.PlatformNode,
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0(exports) {
    // /entry.js
    __export(exports, {
      fs: () => fs,
      readFileSync: () => fs2.readFileSync
    });
    const fs = require("fs");
    const fs2 = require("fs");
  }
}, 0);
`,
		},
	})
}

func TestExportWildcardFSNode(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				export * from 'fs'
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		resolveOptions: resolver.ResolveOptions{
			Platform: resolver.PlatformNode,
		},
		expectedCompileLog: "/entry.js: error: Wildcard exports are not supported for this module\n",
	})
}

func TestMinifiedBundleES6(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {foo} from './a'
				console.log(foo())
			`,
			"/a.js": `
				export function foo() {
					return 123
				}
				foo()
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling:   true,
			MangleSyntax: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:        true,
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({1(){function a(){return 123}a();console.log(a())}},1);
`,
		},
	})
}

func TestMinifiedBundleCommonJS(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				const {foo} = require('./a')
				console.log(foo(), require('./j.json'))
			`,
			"/a.js": `
				exports.foo = function() {
					return 123
				}
			`,
			"/j.json": `
				{"test": true}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling:   true,
			MangleSyntax: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:        true,
			RemoveWhitespace:  true,
			MinifyIdentifiers: true,
			AbsOutputFile:     "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({0(a){a.foo=function(){return 123}},2(b,a){a.exports={test:!0}},1(){const{foo:b}=f$(0);console.log(b(),f$(2))}},1);
`,
		},
	})
}

func TestMinifiedBundleEndingWithImportantSemicolon(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				while(foo()); // This must not be stripped
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:       true,
			RemoveWhitespace: true,
			AbsOutputFile:    "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({0(){while(foo());}},0);
`,
		},
	})
}

func TestOptionalCatchNameCollisionNoBundle(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				try {}
				catch { var e, e2 }
				var e3
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: false,
			Target:     parser.ES2018,
		},
		bundleOptions: BundleOptions{
			IsBundling:    false,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `try {
} catch (e4) {
  var e, e2;
}
var e3;
`,
		},
	})
}

func TestTopLevelReturn(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				import {foo} from './foo'
				foo()
			`,
			"/foo.js": `
				// Top-level return must force CommonJS mode
				if (Math.random() < 0.5) return

				export function foo() {}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  1(exports) {
    // /foo.js
    __export(exports, {
      foo: () => foo
    });
    if (Math.random() < 0.5)
      return;
    function foo() {
    }
  },

  0() {
    // /entry.js
    const foo = __import(1 /* ./foo */);
    foo.foo();
  }
}, 0);
`,
		},
	})
}

func TestThisOutsideFunction(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				console.log(this)
				console.log((x = this) => this)
				console.log({x: this})
				console.log(class extends this.foo {})
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0(exports) {
    // /entry.js
    console.log(exports);
    console.log((x = exports) => exports);
    console.log({
      x: exports
    });
    console.log(class extends exports.foo {
    });
  }
}, 0);
`,
		},
	})
}

func TestThisInsideFunction(t *testing.T) {
	expectBundled(t, bundled{
		files: map[string]string{
			"/entry.js": `
				function foo(x = this) { console.log(this) }
				const obj = {
					foo(x = this) { console.log(this) }
				}
				class Foo {
					x = this
					static y = this.z
					foo(x = this) { console.log(this) }
					static bar(x = this) { console.log(this) }
				}
			`,
		},
		entryPaths: []string{"/entry.js"},
		parseOptions: parser.ParseOptions{
			IsBundling: true,
		},
		bundleOptions: BundleOptions{
			IsBundling:    true,
			AbsOutputFile: "/out.js",
		},
		expected: map[string]string{
			"/out.js": `bootstrap({
  0() {
    // /entry.js
    function foo(x = this) {
      console.log(this);
    }
    const obj = {
      foo(x = this) {
        console.log(this);
      }
    };
    class Foo {
      x = this;
      static y = this.z;
      foo(x = this) {
        console.log(this);
      }
      static bar(x = this) {
        console.log(this);
      }
    }
  }
}, 0);
`,
		},
	})
}
