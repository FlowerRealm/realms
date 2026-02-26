const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { pipeline } = require("node:stream/promises");
const { Readable } = require("node:stream");

function mapPlatformArch() {
  const platform = process.platform;
  const arch = process.arch;

  if (platform === "darwin") {
    if (arch === "arm64") return { goos: "darwin", assetArch: "arm64", ext: "" };
    if (arch === "x64") return { goos: "darwin", assetArch: "amd64", ext: "" };
  }

  if (platform === "linux") {
    if (arch === "arm64") return { goos: "linux", assetArch: "arm64", ext: "" };
    if (arch === "x64") return { goos: "linux", assetArch: "amd64", ext: "" };
    if (arch === "ia32") return { goos: "linux", assetArch: "386", ext: "" };
    if (arch === "arm") return { goos: "linux", assetArch: "armv7", ext: "" };
    if (arch === "ppc64") {
      if (os.endianness() === "LE") return { goos: "linux", assetArch: "ppc64le", ext: "" };
      return null;
    }
    if (arch === "s390x") return { goos: "linux", assetArch: "s390x", ext: "" };
    if (arch === "riscv64") return { goos: "linux", assetArch: "riscv64", ext: "" };
  }

  if (platform === "win32") {
    if (arch === "arm64") return { goos: "windows", assetArch: "arm64", ext: ".exe" };
    if (arch === "x64") return { goos: "windows", assetArch: "amd64", ext: ".exe" };
    if (arch === "ia32") return { goos: "windows", assetArch: "386", ext: ".exe" };
  }

  if (platform === "freebsd") {
    if (arch === "arm64") return { goos: "freebsd", assetArch: "arm64", ext: "" };
    if (arch === "x64") return { goos: "freebsd", assetArch: "amd64", ext: "" };
    if (arch === "ia32") return { goos: "freebsd", assetArch: "386", ext: "" };
    if (arch === "arm") return { goos: "freebsd", assetArch: "armv7", ext: "" };
  }

  if (platform === "openbsd") {
    if (arch === "arm64") return { goos: "openbsd", assetArch: "arm64", ext: "" };
    if (arch === "x64") return { goos: "openbsd", assetArch: "amd64", ext: "" };
  }

  return null;
}

function sha256File(filePath) {
  return new Promise((resolve, reject) => {
    const hash = crypto.createHash("sha256");
    const input = fs.createReadStream(filePath);
    input.on("error", reject);
    input.on("data", (chunk) => hash.update(chunk));
    input.on("end", () => resolve(hash.digest("hex")));
  });
}

async function downloadToFile(url, filePath) {
  const res = await fetch(url, {
    redirect: "follow",
    headers: {
      "user-agent": `realms-npm/${process.version} (${process.platform}; ${process.arch})`,
    },
  });
  if (!res.ok) {
    throw new Error(`下载失败: ${res.status} ${res.statusText} (${url})`);
  }
  if (!res.body) {
    throw new Error(`下载失败: response body 为空 (${url})`);
  }
  await pipeline(Readable.fromWeb(res.body), fs.createWriteStream(filePath));
}

function parseChecksums(checksumsText) {
  const m = new Map();
  for (const rawLine of checksumsText.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line) continue;
    const parts = line.split(/\s+/);
    if (parts.length < 2) continue;
    const sum = parts[0].toLowerCase();
    const file = parts[parts.length - 1];
    if (/^[0-9a-f]{64}$/.test(sum) && file) {
      m.set(file, sum);
    }
  }
  return m;
}

async function main() {
  if ((process.env.REALMS_APP_SKIP_DOWNLOAD || "").trim() === "1") {
    return;
  }

  const mapped = mapPlatformArch();
  if (!mapped) {
    console.error(`[realms] 当前平台不受支持: ${process.platform}/${process.arch}`);
    process.exit(1);
  }

  const packageRoot = path.resolve(__dirname, "..");
  const pkg = require(path.join(packageRoot, "package.json"));

  const configuredVersion = (process.env.REALMS_APP_VERSION || "").trim();
  const version = configuredVersion || pkg.version;
  if (version === "0.0.0" && !configuredVersion) {
    console.error("[realms] 包版本为 0.0.0，无法自动推断要下载的 Release 版本。");
    console.error("- 请设置 REALMS_APP_VERSION（例如 v0.16.0），或使用正式发布的 npm 版本。");
    process.exit(1);
  }

  const tag = version.startsWith("v") ? version : `v${version}`;
  const base = (process.env.REALMS_APP_BASE_URL || "https://github.com/FlowerRealm/realms/releases/download").replace(/\/$/, "");

  const vendorDir = path.join(packageRoot, "vendor");
  fs.mkdirSync(vendorDir, { recursive: true });

  const asset = `realms-app_${tag}_${mapped.goos}_${mapped.assetArch}${mapped.ext}`;
  const dest = path.join(vendorDir, asset);

  if (fs.existsSync(dest)) {
    return;
  }

  const tmp = path.join(vendorDir, `${asset}.tmp-${process.pid}-${Date.now()}-${crypto.randomUUID?.() || "r"}`);
  const url = `${base}/${tag}/${asset}`;
  const checksumsUrl = `${base}/${tag}/realms-app_${tag}_checksums.txt`;

  let expected = null;
  if ((process.env.REALMS_APP_SKIP_CHECKSUM || "").trim() !== "1") {
    const checksumsRes = await fetch(checksumsUrl, { redirect: "follow" });
    if (!checksumsRes.ok) {
      throw new Error(`获取 checksums 失败: ${checksumsRes.status} ${checksumsRes.statusText} (${checksumsUrl})`);
    }
    const checksumsText = await checksumsRes.text();
    const map = parseChecksums(checksumsText);
    expected = map.get(asset) || null;
    if (!expected) {
      throw new Error(`checksums 中未找到资产: ${asset}`);
    }
  }

  await downloadToFile(url, tmp);

  if (expected) {
    const actual = await sha256File(tmp);
    if (actual !== expected) {
      throw new Error(`sha256 校验失败: expected=${expected} actual=${actual} asset=${asset}`);
    }
  }

  fs.renameSync(tmp, dest);

  if (process.platform !== "win32") {
    fs.chmodSync(dest, 0o755);
  }

  console.log(`[realms] 已安装 realms-app: ${asset}`);
}

main().catch((err) => {
  const msg = err && err.stack ? err.stack : String(err);
  console.error(`[realms] postinstall 失败: ${msg}`);
  console.error("- 你可以：");
  console.error("  1) 重试安装：npm install -g realms");
  console.error("  2) 配置镜像：REALMS_APP_BASE_URL=...（默认 GitHub Releases）");
  console.error("  3) 或跳过下载：REALMS_APP_SKIP_DOWNLOAD=1");
  process.exit(1);
});
