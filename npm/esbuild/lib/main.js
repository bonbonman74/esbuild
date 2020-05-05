const child_process = require('child_process');
const path = require('path');
const os = require('os');

function binPath() {
  if ((process.platform === 'linux' || process.platform === 'darwin') && os.arch() === 'x64') {
    return path.join(__dirname, '..', 'bin', 'esbuild');
  }

  if (process.platform === 'win32' && os.arch() === 'x64') {
    return path.join(__dirname, '..', '.install', 'node_modules', 'esbuild-windows-64', 'esbuild.exe');
  }

  throw new Error(`Unsupported platform: ${process.platform} ${os.arch()}`);
}

exports.build = options => {
  return new Promise((resolve, reject) => {
    const flags = [`--error-limit=${options.errorLimit || 0}`];
    const stdio = options.stdio;

    if (options.name) flags.push(`--name=${options.name}`);
    if (options.bundle) flags.push('--bundle');
    if (options.outfile) flags.push(`--outfile=${options.outfile}`);
    if (options.outdir) flags.push(`--outdir=${options.outdir}`);
    if (options.sourcemap) flags.push('--sourcemap');
    if (options.target) flags.push(`--target=${options.target}`);
    if (options.platform) flags.push(`--platform=${options.platform}`);
    if (options.format) flags.push(`--format=${options.format}`);
    if (options.color) flags.push(`--color=${options.color}`);
    if (options.external) for (const name of options.external) flags.push(`--external:${name}`);

    if (options.minify) flags.push('--minify');
    if (options.minifySyntax) flags.push('--minify-syntax');
    if (options.minifyWhitespace) flags.push('--minify-whitespace');
    if (options.minifyIdentifiers) flags.push('--minify-identifiers');

    if (options.jsxFactory) flags.push(`--jsx-factory=${options.jsxFactory}`);
    if (options.jsxFragment) flags.push(`--jsx-fragment=${options.jsxFragment}`);
    if (options.define) for (const key in options.define) flags.push(`--define:${key}=${options.define[key]}`);
    if (options.loader) for (const ext in options.loader) flags.push(`--loader:${ext}=${options.loader[ext]}`);

    for (const entryPoint of options.entryPoints) {
      if (entryPoint.startsWith('-')) throw new Error(`Invalid entry point: ${entryPoint}`);
      flags.push(entryPoint);
    }

    const child = child_process.spawn(binPath(), flags, { cwd: process.cwd(), windowsHide: true, stdio });
    child.on('error', error => reject(error));

    // The stderr pipe won't be available if "stdio" is set to "inherit"
    const stderrChunks = [];
    if (child.stderr) child.stderr.on('data', chunk => stderrChunks.push(chunk));

    child.on('close', code => {
      const fullRegex = /^(.+):(\d+):(\d+): (warning|error): (.+)$/;
      const smallRegex = /^(warning|error): (.+)$/;
      const errors = [];
      const warnings = [];
      const stderr = Buffer.concat(stderrChunks).toString();

      for (const line of stderr.split('\n')) {
        let match = fullRegex.exec(line);
        if (match) {
          const [, file, line, column, kind, text] = match;
          (kind === 'error' ? errors : warnings).push({ text, location: { file, line: +line, column: +column } });
        }

        else {
          match = smallRegex.exec(line);
          if (match) {
            const [, kind, text] = match;
            (kind === 'error' ? errors : warnings).push({ text, location: null });
          }
        }
      }

      if (errors.length === 0 && code === 0) {
        resolve({ stderr, warnings });
      }

      else {
        // The error array will be empty if "stdio" is set to "inherit"
        const summary = errors.length < 1 ? '' : ` with ${errors.length} error${errors.length < 2 ? '' : 's'}`;
        const error = new Error(`Build failed${summary}`);
        error.stderr = stderr;
        error.errors = errors;
        error.warnings = warnings;
        reject(error);
      }
    });
  });
}

exports.startService = () => {
  return new Promise((resolve, reject) => {
    const child = child_process.spawn(binPath(), ['--service'], {
      windowsHide: true,
      stdio: ['pipe', 'pipe', 'inherit'],
    });
    child.on('error', error => reject(error));
    const requests = new Map();
    let isClosed = false;
    let nextID = 0;

    // Use a long-lived buffer to store stdout data
    let stdout = Buffer.alloc(4096);
    let stdoutUsed = 0;
    child.stdout.on('data', chunk => {
      // Append the chunk to the stdout buffer, growing it as necessary
      const limit = stdoutUsed + chunk.length;
      if (limit > stdout.length) {
        let swap = Buffer.alloc(limit * 2);
        swap.set(stdout);
        stdout = swap;
      }
      stdout.set(chunk, stdoutUsed);
      stdoutUsed += chunk.length;

      // Process all complete (i.e. not partial) responses
      let offset = 0;
      while (offset + 4 <= stdoutUsed) {
        const length = stdout.readUInt32LE(offset);
        if (offset + 4 + length > stdoutUsed) {
          break;
        }
        offset += 4;
        handleResponse(stdout.slice(offset, offset + length));
        offset += length;
      }
      if (offset > 0) {
        stdout.set(stdout.slice(offset));
        stdoutUsed -= offset;
      }
    });

    child.on('close', () => {
      // When the process is closed, fail all pending requests
      isClosed = true;
      for (const { reject } of requests.values()) {
        reject(new Error('The service was stopped'));
      }
      requests.clear();
    });

    function sendRequest(request) {
      return new Promise((resolve, reject) => {
        if (isClosed) throw new Error('The service is no longer running');

        // Allocate an id for this request
        const id = (nextID++).toString();
        requests.set(id, { resolve, reject });

        // Figure out how long the request will be
        const argBuffers = [Buffer.from(id)];
        let length = 4 + 4 + 4 + argBuffers[0].length;
        for (const arg of request) {
          const argBuffer = Buffer.from(arg);
          argBuffers.push(argBuffer);
          length += 4 + argBuffer.length;
        }

        // Write out the request message
        const bytes = Buffer.alloc(length);
        let offset = 0;
        const writeUint32 = value => {
          bytes.writeUInt32LE(value, offset);
          offset += 4;
        };
        writeUint32(length - 4);
        writeUint32(argBuffers.length);
        for (const argBuffer of argBuffers) {
          writeUint32(argBuffer.length);
          bytes.set(argBuffer, offset);
          offset += argBuffer.length;
        }
        child.stdin.write(bytes);
      });
    }

    function handleResponse(bytes) {
      let offset = 0;
      const eat = n => {
        offset += n;
        if (offset > bytes.length) throw new Error('Invalid message');
        return offset - n;
      };
      const count = bytes.readUInt32LE(eat(4));
      const response = {};
      let id;

      // Parse the response into a map
      for (let i = 0; i < count; i++) {
        const keyLength = bytes.readUInt32LE(eat(4));
        const key = bytes.slice(offset, eat(keyLength) + keyLength).toString();
        const valueLength = bytes.readUInt32LE(eat(4));
        const value = bytes.slice(offset, eat(valueLength) + valueLength);
        if (key === 'id') id = value.toString();
        else response[key] = value.toString();
      }

      // Dispatch the response
      if (!id) throw new Error('Invalid message');
      const { resolve, reject } = requests.get(id);
      requests.delete(id);
      if (response.error) reject(new Error(response.error));
      else resolve(response);
    }

    // Send an initial ping before resolving with the service object to make
    // sure the service is up and running
    sendRequest(['ping']).then(() => resolve({
      stop() {
        child.kill();
      },
    }));
  });
};
