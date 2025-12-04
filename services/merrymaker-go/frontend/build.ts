import { createHash } from "node:crypto";
import { cp, mkdir, readdir, readFile, rename, rm, stat, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { build } from "bun";

interface NodeError extends Error {
	code?: string;
}

const CONFIG = {
	paths: {
		styles: "./styles",
		outputCss: "./public/css",
		output: "./static",
	},
	css: {
		entry: "./styles/index-modular.css",
		output: "styles.css",
		criticalEntry: "./styles/critical.css",
		criticalOutput: "critical.css",
	},
	// Development mode flag - can be set via environment variable
	isDev: process.env.NODE_ENV === "development" || process.env.DEV === "true",
};

async function buildCSS() {
	await mkdir(CONFIG.paths.outputCss, { recursive: true });

	// Build main CSS
	await build({
		entrypoints: [CONFIG.css.entry],
		outdir: CONFIG.paths.outputCss,
		sourcemap: "external",
		target: "browser",
		minify: !CONFIG.isDev,
		naming: { entry: CONFIG.css.output },
	});

	// Build critical CSS (always minified for inlining)
	await build({
		entrypoints: [CONFIG.css.criticalEntry],
		outdir: CONFIG.paths.outputCss,
		sourcemap: "none",
		target: "browser",
		minify: true,
		naming: { entry: CONFIG.css.criticalOutput },
	});

	if (CONFIG.isDev) {
		console.log("✓ Built CSS in development mode (unminified)");
	} else {
		console.log("✓ Built CSS in production mode (minified)");
	}
	console.log("✓ Built critical CSS (minified for inlining)");
}

async function copyPublicFiles() {
	const source = "./public";
	const destination = CONFIG.paths.output;
	try {
		await mkdir(destination, { recursive: true });
		await cp(source, destination, { recursive: true });
		console.log("✓ Copied public assets");
	} catch (error) {
		// Ignore if public directory doesn't exist
		if (error instanceof Error && "code" in error && error.code === "ENOENT") {
			console.log("No 'public' directory found to copy, skipping.");
			return;
		}
		console.error("Failed to copy public assets:", error);
		throw error; // Propagate other errors
	}
}

async function copyFontFiles() {
	// Copy only the Inter font weights we actually use (400, 500, 600) in latin subset
	const fontsSource = "./node_modules/@fontsource/inter/files";
	const fontsDestination = join(CONFIG.paths.output, "fonts");

	const fontFiles = [
		"inter-latin-400-normal.woff2",
		"inter-latin-500-normal.woff2",
		"inter-latin-600-normal.woff2",
	];

	try {
		await mkdir(fontsDestination, { recursive: true });

		for (const file of fontFiles) {
			const sourcePath = join(fontsSource, file);
			const destPath = join(fontsDestination, file);
			await cp(sourcePath, destPath);
		}

		console.log("✓ Copied font files");
	} catch (error) {
		console.error("Failed to copy font files:", error);
		throw error;
	}
}

async function buildJS() {
	// Bundle/minify the app's public JS entrypoints to /static/js with stable filenames.
	// Tree-shaking is automatically applied by the bundler when building ESM with minification.
	// splitting: false ensures single-file outputs with stable names for content hashing.
	const entrypoints = [
		"./public/js/app.js",
		"./public/js/source_form.js",
		"./public/js/alert_sink_form.js",
		"./public/js/job_status.js",
	];
	await build({
		entrypoints,
		outdir: `${CONFIG.paths.output}/js`,
		sourcemap: "external",
		target: "browser",
		minify: true,
		naming: { entry: "[name].js" },
		splitting: false,
	});
	console.log("✓ Built JS bundles");
}

async function buildUILibs() {
	// Split vendor libraries for better code splitting:
	// - vendor.core.js: htmx + lucide (needed on every page)
	// - vendor.jmespath.js: jmespath only (needed only on alert sink forms)
	// Build both in parallel to reduce build time

	await Promise.all([
		// Build core UI libs (htmx + lucide)
		build({
			entrypoints: ["./vendor/ui_core.ts"],
			outdir: `${CONFIG.paths.output}/js`,
			sourcemap: "external",
			target: "browser",
			minify: true,
			naming: { entry: "vendor.core.js" },
			format: "iife",
			splitting: false,
		}),
		// Build jmespath separately for on-demand loading
		build({
			entrypoints: ["./vendor/ui_jmespath.ts"],
			outdir: `${CONFIG.paths.output}/js`,
			sourcemap: "external",
			target: "browser",
			minify: true,
			naming: { entry: "vendor.jmespath.js" },
			format: "iife",
			splitting: false,
		}),
	]);
	console.log("✓ Built vendor libraries");
}

// Generate content hash for a file
async function generateFileHash(filePath: string): Promise<string> {
	const file = Bun.file(filePath);
	const buffer = await file.arrayBuffer();
	const hash = createHash("sha256");
	hash.update(new Uint8Array(buffer));
	return hash.digest("hex").substring(0, 8); // Use first 8 chars
}

// Rename files with content hashes and generate manifest
// Update sourceMappingURL in asset and "file" field in its map to reference hashed names
async function updateSourceMapReferences(
	assetPath: string,
	mapPath: string,
	hashedAssetName: string,
) {
	try {
		let content = await readFile(assetPath, "utf8");
		const mapFileName = `${hashedAssetName}.map`;
		// Replace JS style source map comment
		content = content.replace(/\/\/# sourceMappingURL=.*$/m, `//# sourceMappingURL=${mapFileName}`);
		// Replace CSS style source map comment
		content = content.replace(
			/\/\*# sourceMappingURL=.*?\*\//m,
			`/*# sourceMappingURL=${mapFileName} */`,
		);
		await writeFile(assetPath, content);
	} catch (err) {
		if ((err as NodeError)?.code !== "ENOENT") {
			console.warn("Failed to update source map reference in asset:", err);
		}
	}
	try {
		const raw = await readFile(mapPath, "utf8");
		const map = JSON.parse(raw);
		map.file = hashedAssetName;
		await writeFile(mapPath, JSON.stringify(map));
	} catch (err) {
		if ((err as NodeError)?.code !== "ENOENT") {
			console.warn("Failed to update source map file reference:", err);
		}
	}
}

async function addContentHashes(): Promise<Record<string, string>> {
	const manifest: Record<string, string> = {};
	const staticDir = CONFIG.paths.output;

	// Process CSS files
	const cssDir = join(staticDir, "css");
	try {
		const cssFiles = await readdir(cssDir);
		for (const file of cssFiles) {
			if (file.endsWith(".css") && !file.includes(".map")) {
				// Skip critical.css - it's loaded by Go at runtime, not referenced in HTML
				if (file === "critical.css") {
					continue;
				}

				const filePath = join(cssDir, file);
				const hash = await generateFileHash(filePath);
				const name = file.replace(".css", "");
				const hashedName = `${name}.${hash}.css`;
				const hashedPath = join(cssDir, hashedName);

				// Atomically rename the file
				await rename(filePath, hashedPath);

				// Also handle source map if it exists and update references
				const mapFile = `${file}.map`;
				const mapPath = join(cssDir, mapFile);
				try {
					await stat(mapPath);
					const hashedMapPath = join(cssDir, `${hashedName}.map`);
					await rename(mapPath, hashedMapPath);
					await updateSourceMapReferences(hashedPath, hashedMapPath, hashedName);
				} catch {
					// Source map doesn't exist, ignore
				}

				manifest[`css/${name}.css`] = `css/${hashedName}`;
			}
		}
	} catch {
		// CSS directory doesn't exist, ignore
	}

	// Process JS files
	const jsDir = join(staticDir, "js");
	try {
		const jsFiles = await readdir(jsDir);
		for (const file of jsFiles) {
			if (file.endsWith(".js") && !file.includes(".map")) {
				const filePath = join(jsDir, file);
				const hash = await generateFileHash(filePath);
				const name = file.replace(".js", "");
				const hashedName = `${name}.${hash}.js`;
				const hashedPath = join(jsDir, hashedName);

				// Atomically rename the file
				await rename(filePath, hashedPath);

				// Also handle source map if it exists and update references
				const mapFile = `${file}.map`;
				const mapPath = join(jsDir, mapFile);
				let hashedMapPath: string | null = null;
				try {
					await stat(mapPath);
					hashedMapPath = join(jsDir, `${hashedName}.map`);
					await rename(mapPath, hashedMapPath);
					await updateSourceMapReferences(hashedPath, hashedMapPath, hashedName);
				} catch {
					// Source map doesn't exist, ignore
				}

				manifest[`js/${name}.js`] = `js/${hashedName}`;
			}
		}
	} catch {
		// JS directory doesn't exist, ignore
	}

	return manifest;
}

// Write manifest.json
async function writeManifest(manifest: Record<string, string>) {
	const manifestPath = join(CONFIG.paths.output, "manifest.json");
	await writeFile(manifestPath, JSON.stringify(manifest, null, 2));
	console.log("✓ Generated asset manifest");
}

// Inline critical CSS into layout template
// Critical CSS is now loaded by Go at runtime via the {{criticalCSS}} template function.
// No need to inline it at build time - just ensure it's built and minified.

async function main() {
	console.log("Starting build process...");
	await rm(CONFIG.paths.output, {
		recursive: true,
		force: true,
	});

	// Build CSS into public, copy all public assets to /static,
	// then bundle JS so it overwrites copied JS with minified builds
	await buildCSS();
	await copyPublicFiles();
	await copyFontFiles();
	await buildJS();
	await buildUILibs();

	// Add content hashes and generate manifest
	const manifest = await addContentHashes();
	await writeManifest(manifest);

	console.log("✓ Build complete - critical CSS will be loaded by Go at runtime");
}

main().catch((err) => {
	console.error(err);
	process.exit(1);
});
