// Produces the reference output the differential harness compares against.
//
// This is the only place Node appears in the project, and it is deliberate:
// the reference implementation is the definition of correct, so it is kept in
// the loop for verification even though it has been removed from the build.
//
// Versions are pinned exactly. An unpinned reference would let an upstream
// release change the definition of correct underneath a green build.

import { transformAsync } from "@babel/core";
import solid from "babel-preset-solid";
import { readdir, readFile, writeFile, mkdir } from "node:fs/promises";
import { dirname, join, basename } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const corpus = join(here, "..", "testdata", "corpus");
const expected = join(here, "..", "testdata", "expected");

await mkdir(expected, { recursive: true });

const files = (await readdir(corpus)).filter((f) => f.endsWith(".tsx")).sort();
if (files.length === 0) {
  console.error("no fixtures found in", corpus);
  process.exit(1);
}

let failed = 0;
for (const file of files) {
  const src = await readFile(join(corpus, file), "utf8");
  try {
    const out = await transformAsync(src, {
      filename: file,
      babelrc: false,
      configFile: false,
      // generate: "dom" is the client renderer, which is what go-solid emits.
      // hydratable stays off until the compiler grows hydration support.
      presets: [[solid, { generate: "dom", hydratable: false }]],
      // Comments are dropped so that the goldens diff cleanly; the harness
      // strips them anyway, and keeping them adds churn for no signal.
      comments: true,
    });
    const target = join(expected, basename(file, ".tsx") + ".js");
    await writeFile(target, out.code + "\n", "utf8");
    console.log("recorded", basename(target));
  } catch (err) {
    console.error(`FAILED ${file}: ${err.message}`);
    failed++;
  }
}

if (failed > 0) {
  console.error(`${failed} fixture(s) failed to transform`);
  process.exit(1);
}
console.log(`recorded ${files.length} fixture(s)`);
