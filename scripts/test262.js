const fs = require('fs');
const path = require('path');
const jsYaml = require('js-yaml');
const { installForTests } = require('./esbuild');
const test262Dir = path.join(__dirname, '..', 'demo', 'test262', 'test');

function findFiles() {
  function visit(dir) {
    for (const entry of fs.readdirSync(dir)) {
      const fullEntry = path.join(dir, entry);
      const stats = fs.statSync(fullEntry);
      if (stats.isDirectory()) {
        visit(fullEntry);
      } else if (stats.isFile() && entry.endsWith('.js') && !entry.endsWith('_FIXTURE.js')) {
        files.push(fullEntry);
      }
    }
  }

  const files = [];
  visit(test262Dir);
  return files;
}

function formatErrors(content, error) {
  if (error.errors) {
    return error.errors.map(({ text, location }) => {
      if (location) {
        const { line, column } = location;
        const contentLine = content.split(/(?:\r\n|\n|\r|\u2028|\u2029)/g)[line - 1];
        return `<stdin>:${line}:${column}: error: ${text}\n${contentLine}\n${' '.repeat(column)}^`;
      }
      return `error: ${text}`;
    }).join('\n');
  }
  return error + '';
}

async function main() {
  const esbuild = installForTests();
  const files = findFiles();
  let runCount = 0;
  let shouldHavePassed = 0;
  let shouldHaveFailed = 0;
  let reparseCount = 0;
  let reprintCount = 0;
  let minifyCount = 0;

  async function esbuildFile(input, options) {
    try {
      const { code } = await esbuild.transform(input, options);
      return { success: true, output: code };
    } catch (error) {
      return { success: false, error };
    }
  }

  const skipList = [
    // Skip these tests because we deliberately support top-level return (input
    // files are treated as CommonJS and/or ESM but never as global code, and
    // top-level return is allowed in CommonJS)
    'language/statements/return/S12.9_A1_T1.js', // Checking if execution of "return" with no function fails
    'language/statements/return/S12.9_A1_T10.js', // Checking if execution of "return (0)" with no function fails
    'language/statements/return/S12.9_A1_T2.js', // Checking if execution of "return x" with no function fails
    'language/statements/return/S12.9_A1_T3.js', // Checking if execution of "return" within "try" statement fails
    'language/statements/return/S12.9_A1_T4.js', // Checking if execution of "return" with no function fails
    'language/statements/return/S12.9_A1_T5.js', // Checking if execution of "return" with no function, placed into a Block, fails
    'language/statements/return/S12.9_A1_T6.js', // Checking if execution of "return" with no function, placed into a loop, fails
    'language/statements/return/S12.9_A1_T7.js', // Checking if execution of "return x" with no function, placed inside Block, fails
    'language/statements/return/S12.9_A1_T8.js', // Checking if execution of "return x" with no function, placed into a loop, fails
    'language/statements/return/S12.9_A1_T9.js', // Checking if execution of "return", placed into a catch Block, fails
    'language/global-code/return.js',      // ReturnStatement may not be used directly within global code

    // Skip these tests because we deliberately support parsing top-level await
    // in all files. Files containing top-level await are always interpreted as
    // ESM, never as CommonJS.
    'language/expressions/assignmenttargettype/simple-basic-identifierreference-await.js', // IdentifierReference  await Return simple. (Simple Direct assignment)
    'language/expressions/await/await-BindingIdentifier-in-global.js', // Object literal shorthands are limited to valid identifier references. await is valid in non-module strict mode code.
    'language/expressions/await/await-in-global.js', // Await is allowed as a binding identifier in global scope
    'language/expressions/await/await-in-nested-function.js', // Await is an identifier in global scope
    'language/expressions/await/await-in-nested-generator.js', // Await is allowed as an identifier in functions nested in async functions
    'language/expressions/class/class-name-ident-await-escaped.js', // Await is allowed as an identifier in generator functions nested in async functions
    'language/expressions/class/class-name-ident-await.js', // `await` with escape sequence is a valid class-name identifier.
    'language/expressions/dynamic-import/assignment-expression/await-identifier.js', // `await` is a valid class-name identifier.
    'language/expressions/object/identifier-shorthand-await-strict-mode.js', // Dynamic Import receives an AssignmentExpression (IdentifierReference: await)
    'language/module-code/top-level-await/new-await-script-code.js', // await is not a keyword in script code
    'language/reserved-words/await-script.js', // The `await` token is permitted as an identifier in script code
    'language/statements/class/class-name-ident-await-escaped.js', // `await` with escape sequence is a valid class-name identifier.
    'language/statements/class/class-name-ident-await.js', // `await` is a valid class-name identifier.
    'language/statements/labeled/value-await-non-module-escaped.js', // `await` is not a reserved identifier in non-module code and may be used as a label.
    'language/statements/labeled/value-await-non-module.js', // `await` is not a reserved identifier in non-module code and may be used as a label.

    // Skip these tests because we don't currently validate the contents of
    // regular expressions. We could do this but it's not necessary to parse
    // JavaScript successfully since we parse enough of it to be able to
    // determine where the regular expression ends (just "\" and "[]" pairs).
    'language/literals/regexp/early-err-pattern.js', // It is a Syntax Error if BodyText of RegularExpressionLiteral cannot be recognized using the goal symbol Pattern of the ECMAScript RegExp grammar specified in #sec-patterns.
    'language/literals/regexp/invalid-braced-quantifier-exact.js', // Braced quantifier in an Atom position (exact count)
    'language/literals/regexp/invalid-braced-quantifier-lower.js', // Braced quantifier in an Atom position (lower-bounds)
    'language/literals/regexp/invalid-braced-quantifier-range.js', // Braced quantifier in an Atom position (range)
    'language/literals/regexp/invalid-optional-lookbehind.js', // Lookbehinds are not treated as a QuantifiableAssertion
    'language/literals/regexp/invalid-optional-negative-lookbehind.js', // Lookbehinds are not treated as a QuantifiableAssertion
    'language/literals/regexp/invalid-range-lookbehind.js', // Lookbehinds are not treated as a QuantifiableAssertion
    'language/literals/regexp/invalid-range-negative-lookbehind.js', // Lookbehinds are not treated as a QuantifiableAssertion
    'language/literals/regexp/u-invalid-class-escape.js', // ClassEscape does not recognize "class control" patterns
    'language/literals/regexp/u-invalid-extended-pattern-char.js', // Quantifiable assertions disallowed with `u` flag
    'language/literals/regexp/u-invalid-identity-escape.js', // Support for UnicodeIDContinue in IdentityEscape
    'language/literals/regexp/u-invalid-legacy-octal-escape.js', // Legacy Octal Escape Sequence not supported with `u` flag
    'language/literals/regexp/u-invalid-non-empty-class-ranges-no-dash-a.js', // NonEmptyClassRangesNoDash production cannot specify a multi-character set ("A" parameter)
    'language/literals/regexp/u-invalid-non-empty-class-ranges-no-dash-ab.js', // NonEmptyClassRangesNoDash production cannot specify a multi-character set (both "A" and "B" parameters)
    'language/literals/regexp/u-invalid-non-empty-class-ranges-no-dash-b.js', // NonEmptyClassRangesNoDash production cannot specify a multi-character set ("B" parameter)
    'language/literals/regexp/u-invalid-non-empty-class-ranges.js', // NonEmptyClassRangesNoDash production cannot specify a multi-character set
    'language/literals/regexp/u-invalid-oob-decimal-escape.js', // Out-of-bounds decimal escapes
    'language/literals/regexp/u-invalid-optional-lookahead.js', // Quantifiable assertions disallowed with `u` flag
    'language/literals/regexp/u-invalid-optional-lookbehind.js', // Lookbehinds are not treated as a QuantifiableAssertion
    'language/literals/regexp/u-invalid-optional-negative-lookahead.js', // Quantifiable assertions disallowed with `u` flag
    'language/literals/regexp/u-invalid-optional-negative-lookbehind.js', // Lookbehinds are not treated as a QuantifiableAssertion
    'language/literals/regexp/u-invalid-range-lookahead.js', // Quantifiable assertions disallowed with `u` flag
    'language/literals/regexp/u-invalid-range-lookbehind.js', // Lookbehinds are not treated as a QuantifiableAssertion
    'language/literals/regexp/u-invalid-range-negative-lookahead.js', // Quantifiable assertions disallowed with `u` flag
    'language/literals/regexp/u-invalid-range-negative-lookbehind.js', // Lookbehinds are not treated as a QuantifiableAssertion
    'language/literals/regexp/u-unicode-esc-bounds.js', // Out-of-range value of hexadecimal digits in UnicodeEscapeSequence
    'language/literals/regexp/u-unicode-esc-non-hex.js', // Non-hexadecimal value within the delimiters of a UnicodeEscapeSequence
    'language/literals/regexp/unicode-escape-nls-err.js', // NumericLiteralSeperator disallowed in unicode CodePoint sequence
  ]

  async function processFile(file) {
    let content = fs.readFileSync(file, 'utf8');
    const start = content.indexOf('/*---');
    const end = content.indexOf('---*/');

    if (start < 0 || end < 0) {
      console.warn(`Missing YAML metadata: ${file}`);
      return;
    }

    const yaml = jsYaml.safeLoad(content.slice(start + 5, end));
    const shouldParse = !yaml.negative || yaml.negative.phase === 'runtime';

    if (yaml.features) {
      if (yaml.features.includes('hashbang')) return
      if (yaml.features.includes('regexp-match-indices')) return
      if (yaml.features.includes('regexp-named-groups')) return
      if (yaml.features.includes('regexp-unicode-property-escapes')) return
    }

    if (yaml.flags) {
      if (yaml.flags.includes('onlyStrict')) content = '"use strict";\n' + content
      if (yaml.flags.includes('module')) content = 'export {};\n' + content
    }

    if (skipList.includes(path.relative(test262Dir, file).replace(/\\/g, '/'))) {
      return
    }

    const result = await esbuildFile(content, { minify: false });

    if (result.success !== shouldParse) {
      if (!result.success) shouldHavePassed++;
      else shouldHaveFailed++;
      const text = result.success
        ? (yaml.description || '').trim()
        : formatErrors(content, result.error);
      console.log('\n' + `${file}\n${text}`.replace(/\n/g, '\n  '));
    }

    else if (result.success) {
      const result2 = await esbuildFile(result.output, { minify: false });
      if (!result2.success) {
        console.log(`\n!!! REPARSE ERROR: ${file} !!!`);
        console.log(`${result2.error}`);
        reparseCount++;
      } else if (result2.output !== result.output) {
        console.log(`\n!!! REPRINT ERROR: ${file} !!!`);
        reprintCount++;
      } else {
        const result3 = await esbuildFile(result2.output, { minify: true });
        if (!result3.success) {
          throw new Error('This should have succeeded');
        }
        const result4 = await esbuildFile(result3.output, { minify: true });
        if (!result4.success) {
          console.log(`\n!!! MINIFY ERROR: ${file} !!!`);
          console.log(`${result4.error}`);
          minifyCount++;
        }
      }
    }
    runCount++;
  }

  // Process tests in parallel for speed
  await new Promise((resolve, reject) => {
    let inFlight = 0;
    let i = 0;

    function next() {
      if (i === files.length && inFlight === 0) {
        return resolve();
      }

      while (i < files.length && inFlight < 5) {
        inFlight++;
        processFile(files[i++]).then(() => {
          inFlight--;
          next();
        }, reject);
      }
    }

    next();
  });

  console.log(`tests ran: ${runCount}`);
  console.log(`  tests incorrectly failed: ${shouldHavePassed}`);
  console.log(`  tests incorrectly passed: ${shouldHaveFailed}`);
  console.log(`tests skipped: ${files.length - runCount}`);
  console.log(`reparse failures: ${reparseCount}`);
  console.log(`reprint failures: ${reprintCount}`);
  console.log(`minify failures: ${minifyCount}`);
}

main().catch(e => setTimeout(() => {
  throw e
}));
