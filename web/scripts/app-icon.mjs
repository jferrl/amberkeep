// Renders the mark into the PNGs macOS wants for an application icon.
//
// The favicon follows the interface's theme, because it sits on a browser's tab
// strip. A dock icon cannot: it sits on whatever the person has as a wallpaper, and
// .icns holds no media queries. So it is drawn once, in an amber chosen to read on
// both a pale and a dark desktop — the accent's own hue and chroma, at a lightness
// between the two theme values rather than at either of them.
//
// The drawing is swapped by size the way the brand files intend: the full ant with
// legs and antennae only where they survive, three masses where they do not.
//
// It lives under web/ rather than beside the bundle script because it needs a
// browser, and the browser is the frontend's. Node resolves an import from the
// script's own directory, so a renderer kept elsewhere cannot see it.
import { chromium } from "@playwright/test";
import { readFileSync, mkdirSync, rmSync } from "node:fs";
import { join } from "node:path";

const brand = process.argv[2];
const out = process.argv[3];
const AMBER = "#DA8923";

// macOS asks for each size twice: once at 1x and once as the @2x of the size below.
const wanted = [
  ["icon_16x16.png", 16], ["icon_16x16@2x.png", 32],
  ["icon_32x32.png", 32], ["icon_32x32@2x.png", 64],
  ["icon_128x128.png", 128], ["icon_128x128@2x.png", 256],
  ["icon_256x256.png", 256], ["icon_256x256@2x.png", 512],
  ["icon_512x512.png", 512], ["icon_512x512@2x.png", 1024],
];

const drawing = (size) =>
  size >= 48 ? "mark.svg" : size >= 24 ? "mark-small.svg" : "mark-micro.svg";

rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });

const browser = await chromium.launch();
for (const [name, size] of wanted) {
  const svg = readFileSync(join(brand, drawing(size)), "utf8")
    // The theme query has nothing to answer to here, and the class it sets would
    // leave the shape unpainted.
    .replace(/<style>[\s\S]*?<\/style>/, "")
    .replace(/class="amber"/g, `fill="${AMBER}"`)
    .replace(/width="\d+"/, `width="${size}"`)
    .replace(/height="\d+"/, `height="${size}"`);

  const page = await browser.newPage({ viewport: { width: size, height: size } });
  await page.setContent(
    `<body style="margin:0;background:transparent">${svg}</body>`,
  );
  await page.screenshot({ path: join(out, name), omitBackground: true });
  await page.close();
}
await browser.close();
console.log(`rendered ${String(wanted.length)} sizes into ${out}`);
