import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const srcDir = join(root, "node_modules", "lucide-static", "icons");
const outDir = join(root, "internal", "gateway", "assets", "icons");
const tmpDir = join(root, "node_modules", ".cache", "openlist-tvbox-icons");

const icons = [
  { source: "folder.svg", output: "folder.png", color: "#E8A317" },
  { source: "file-video.svg", output: "video.png", color: "#2F80ED" },
  { source: "file-audio.svg", output: "audio.png", color: "#7B61FF" },
  { source: "file.svg", output: "file.png", color: "#CBD5E1" },
  { source: "list-video.svg", output: "playlist.png", color: "#22C55E" },
  { source: "refresh-cw.svg", output: "refresh.png", color: "#38BDF8" },
];

mkdirSync(outDir, { recursive: true });
mkdirSync(tmpDir, { recursive: true });

for (const icon of icons) {
  const raw = readFileSync(join(srcDir, icon.source), "utf8");
  const paths = raw.match(/<path[\s\S]*?\/>/g)?.join("\n") || "";
  const svg = `<!-- Generated from lucide-static/icons/${icon.source}; see NOTICE.md. -->
<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256" viewBox="0 0 256 256">
  <g transform="translate(48 48) scale(6.6666666667)" fill="none" stroke="${icon.color}" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
${paths}
  </g>
</svg>
`;
  const tmp = join(tmpDir, icon.output.replace(/\.png$/, ".svg"));
  const out = join(outDir, icon.output);
  writeFileSync(tmp, svg);
  execFileSync("magick", ["-background", "none", tmp, "-resize", "256x256", `PNG32:${out}`], { stdio: "inherit" });
}
