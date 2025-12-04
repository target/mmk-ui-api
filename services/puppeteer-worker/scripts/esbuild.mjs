import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import esbuild from "esbuild";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const projectRoot = resolve(__dirname, "..");

const pkg = JSON.parse(
	await readFile(resolve(projectRoot, "package.json"), "utf8"),
);
const externals = Object.keys({
	...(pkg.dependencies || {}),
	...(pkg.peerDependencies || {}),
	...(pkg.optionalDependencies || {}),
});

const rawPlugin = {
	name: "raw",
	setup(build) {
		build.onResolve({ filter: /\?raw$/ }, (args) => {
			const pathNoQuery = args.path.replace(/\?raw$/, "");
			return {
				path: resolve(args.resolveDir, pathNoQuery),
				namespace: "raw",
			};
		});
		build.onLoad({ filter: /.*/, namespace: "raw" }, async (args) => {
			const contents = await readFile(args.path, "utf8");
			return { contents, loader: "text" };
		});
	},
};

const nodeEntries = [
	"src/index.ts",
	"src/examples.ts",
	"src/worker-main.ts",
	// Include core modules to keep dist in sync for tests
	"src/file-capture.ts",
	"src/puppeteer-runner.ts",
	"src/event-monitor.ts",
	"src/event-shipper.ts",
	"src/config-loader.ts",
	"src/config-schema.ts",
	"src/types.ts",
	"src/logger.ts",
	"src/job-client.ts",
	"src/worker-loop.ts",
];

const clientMonitoringEntry = "src/client-monitoring.js";

const nodeBuildConfig = {
	entryPoints: nodeEntries,
	absWorkingDir: projectRoot,
	outdir: "dist",
	platform: "node",
	format: "esm",
	bundle: true,
	sourcemap: true,
	target: "node20",
	external: externals,
	plugins: [rawPlugin],
};

const clientMonitoringConfig = {
	entryPoints: [clientMonitoringEntry],
	absWorkingDir: projectRoot,
	outdir: "dist",
	platform: "browser",
	format: "iife",
	bundle: true,
	sourcemap: true,
	target: ["es2020"],
};

if (process.argv.includes("--watch")) {
	const nodeCtx = await esbuild.context(nodeBuildConfig);
	const clientCtx = await esbuild.context(clientMonitoringConfig);
	await Promise.all([nodeCtx.watch(), clientCtx.watch()]);
	console.log("esbuild watching...");
} else {
	await Promise.all([
		esbuild.build(nodeBuildConfig),
		esbuild.build(clientMonitoringConfig),
	]);
	console.log("esbuild complete");
}
