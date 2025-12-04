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
	resolve(projectRoot, "src/index.ts"),
	resolve(projectRoot, "src/examples.ts"),
	resolve(projectRoot, "src/worker-main.ts"),
	// Include core modules to keep dist in sync for tests
	resolve(projectRoot, "src/file-capture.ts"),
	resolve(projectRoot, "src/puppeteer-runner.ts"),
	resolve(projectRoot, "src/event-monitor.ts"),
	resolve(projectRoot, "src/event-shipper.ts"),
	resolve(projectRoot, "src/config-loader.ts"),
	resolve(projectRoot, "src/config-schema.ts"),
	resolve(projectRoot, "src/types.ts"),
	resolve(projectRoot, "src/logger.ts"),
	resolve(projectRoot, "src/job-client.ts"),
	resolve(projectRoot, "src/worker-loop.ts"),
];

const clientMonitoringEntry = resolve(projectRoot, "src/client-monitoring.js");

const nodeBuildConfig = {
	entryPoints: nodeEntries,
	outdir: resolve(projectRoot, "dist"),
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
	outdir: resolve(projectRoot, "dist"),
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
