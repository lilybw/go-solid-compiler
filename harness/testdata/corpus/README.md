# Differential corpus

Each `.tsx` here is compiled by both babel-preset-solid and this compiler, and
the results compared. `../expected/<name>.js` holds babel's output, committed
so that the Go test suite runs without Node.

Fixtures carry **no type annotations**. Babel strips types and this compiler
deliberately does not — it emits TypeScript for esbuild to strip later — so a
typed fixture would differ for reasons unrelated to the transform. The
transform is purely syntactic and never inspects types, so nothing is lost by
excluding them here. Type handling is esbuild's job and is tested elsewhere.

Adding a fixture: write the `.tsx`, run `npm --prefix ../babel run record`,
commit both files.
