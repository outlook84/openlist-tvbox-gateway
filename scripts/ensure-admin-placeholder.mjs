import { mkdir, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";

const placeholderPath = join("internal", "admin", "assets", "placeholder.txt");
const placeholderText = "This file keeps the embedded admin assets directory non-empty for clean Go builds.\n";

await mkdir(dirname(placeholderPath), { recursive: true });
await writeFile(placeholderPath, placeholderText, "utf8");
