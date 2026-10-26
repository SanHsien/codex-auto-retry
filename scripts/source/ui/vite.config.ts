import { defineConfig, type Plugin } from "vite";
import { viteSingleFile } from "vite-plugin-singlefile";

const zodDynamicFunctionSources = [
  {
    suffix: "/zod/v4/core/doc.js",
    source: `        const F = Function;
        const content = this?.content ?? [\`\`];
        const factory = new F(...Object.keys(this.closed), \`return function (\${this.args.join(", ")}) {\\n\${content.join("\\n")}\\n};\`);
        return factory(...Object.values(this.closed));`,
    replacement: `        throw new Error("Dynamic Zod schema compilation is disabled in the embedded panel.");`,
  },
  {
    suffix: "/zod/v4/core/util.js",
    source: `    try {
        const F = Function;
        new F("");
        return true;
    }
    catch (_) {
        return false;
    }`,
    replacement: `    return false;`,
  },
  {
    suffix: "/zod/v4/core/compile.js",
    source: `    const F = Function;
    const factoryCode = \`return (input) => {\\n\${code}\\n}\`;
    let fn;
    try {
        const factory = new F(...constantNames, factoryCode);
        fn = factory(...constantValues);
    }
    catch (err) {
        // Malformed generated code (or a CSP environment rejecting \`new Function\`) surfaces as a typed error so the global shim falls back to the runtime instead of crashing with a raw SyntaxError/EvalError.
        throw new ZodCompileUnsupportedError(\`this schema (generated code failed to evaluate: \${err.message})\`);
    }
    if (options?.debug) {
        fn.code = fullCode;
    }
    fn.definite = ctx.definite;
    return fn;`,
    replacement: `    throw new ZodCompileUnsupportedError("Dynamic Zod schema compilation is disabled in the embedded panel.");`,
  },
];

function disableZodDynamicFunction(): Plugin {
  return {
    name: "disable-zod-dynamic-function",
    enforce: "pre",
    transform(code, id) {
      const normalizedId = id.replaceAll("\\", "/");
      const match = zodDynamicFunctionSources.find(({ suffix }) => normalizedId.endsWith(suffix));
      if (!match) return null;
      if (!code.includes(match.source)) {
        this.error(`Expected Zod dynamic Function source was not found in ${normalizedId}.`);
      }
      return { code: code.replace(match.source, match.replacement), map: null };
    },
  };
}

function rejectDynamicFunctionBundle(): Plugin {
  const dynamicFunctionPatterns = [
    /\bnew\s+Function\s*\(/,
    /\b(?:const|let|var)\s+[$\w]+\s*=\s*Function\s*;/,
    // Minified aliasing such as `u=Function,` or `u=Function;`.
    /[$\w]\s*=\s*Function\s*[,;]/,
  ];

  return {
    name: "reject-dynamic-function-bundle",
    generateBundle(_, bundle) {
      for (const output of Object.values(bundle)) {
        if (
          output.type === "chunk" &&
          dynamicFunctionPatterns.some((pattern) => pattern.test(output.code))
        ) {
          this.error(`Dynamic Function remained in bundled output: ${output.fileName}`);
        }
      }
    },
  };
}

export default defineConfig({
  plugins: [disableZodDynamicFunction(), viteSingleFile(), rejectDynamicFunctionBundle()],
  build: {
    target: "es2022",
    cssMinify: true,
    minify: "oxc",
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: {
      input: "panel.html",
    },
  },
});
