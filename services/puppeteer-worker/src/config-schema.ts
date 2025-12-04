import type { Config } from "./types.js";

// Minimal, declarative schema DSL and loader helpers (zero deps)
// The schema defines: type, defaults, env binding, and simple constraints.

type Env = Record<string, string | undefined>;

type BoolNode = { kind: "bool"; env?: string; default: boolean };
type NumNode = { kind: "num"; env?: string; default: number; gt?: number };
type StrNode = { kind: "str"; env?: string; default?: string };
type EnumNode<T extends string> = {
	kind: "enm";
	env?: string;
	values: readonly T[];
	default: T;
};

type ArrNode = {
	kind: "arr";
	env?: string;
	sep?: string;
	item: "str";
	default: string[];
};

type AnyObjectNode = { kind: "anyobj"; default: Record<string, unknown> };

type ObjNode<Shape extends Record<string, SchemaNode>> = {
	kind: "obj";
	shape: Shape;
};

interface SchemaShape {
	[key: string]: SchemaNode;
}

type SchemaNode =
	| BoolNode
	| NumNode
	| StrNode
	| EnumNode<string>
	| ArrNode
	| AnyObjectNode
	| ObjNode<SchemaShape>;

// type ShapeOf<T> = T extends ObjNode<infer S> ? S : never;

type ValueOf<N> = N extends BoolNode
	? boolean
	: N extends NumNode
		? number
		: N extends StrNode
			? string | undefined
			: N extends EnumNode<infer T>
				? T
				: N extends ArrNode
					? string[]
					: N extends AnyObjectNode
						? Record<string, unknown>
						: N extends ObjNode<infer S>
							? { [K in keyof S]: ValueOf<S[K]> }
							: never;

export const bool = (opts: { env?: string; default: boolean }): BoolNode => ({
	kind: "bool",
	...opts,
});
export const num = (opts: {
	env?: string;
	default: number;
	gt?: number;
}): NumNode => ({ kind: "num", ...opts });
export const str = (
	opts: { env?: string; default?: string } = {},
): StrNode => ({ kind: "str", ...opts });
export const enm = <T extends string>(
	values: readonly T[],
	opts: { env?: string; default: T },
): EnumNode<T> => ({ kind: "enm", values, ...opts });
export const arr = (
	item: "str",
	opts: { env?: string; sep?: string; default: string[] },
): ArrNode => ({ kind: "arr", item, ...opts });
export const anyobj = (
	opts: { default?: Record<string, unknown> } = {},
): AnyObjectNode => ({ kind: "anyobj", default: opts.default ?? {} });
export const obj = <S extends Record<string, SchemaNode>>(
	shape: S,
): ObjNode<S> => ({ kind: "obj", shape });

// Parsing helpers
function parseBool(v: unknown): boolean | undefined {
	if (typeof v === "boolean") return v;
	if (typeof v === "string") {
		const s = v.trim().toLowerCase();
		if (["true", "1", "yes", "on"].includes(s)) return true;
		if (["false", "0", "no", "off"].includes(s)) return false;
	}
	return undefined;
}

function parseNum(v: unknown): number | undefined {
	if (typeof v === "number" && Number.isFinite(v)) return v;
	if (typeof v === "string") {
		const n = Number(v.trim());
		if (Number.isFinite(n)) return n;
	}
	return undefined;
}

function parseStr(v: unknown): string | undefined {
	if (typeof v === "string") return v;
	return undefined;
}

function parseStrArray(v: unknown, sep = ","): string[] | undefined {
	if (Array.isArray(v))
		return v
			.map((x) => String(x))
			.map((s) => s.trim())
			.filter(Boolean);
	if (typeof v === "string")
		return v
			.split(sep)
			.map((s) => s.trim())
			.filter(Boolean);
	return undefined;
}

function isPlainObject(v: unknown): v is Record<string, unknown> {
	return v !== null && typeof v === "object" && !Array.isArray(v);
}

// Core evaluation
function fromYaml<N extends SchemaNode>(
	node: N,
	yamlValue: unknown,
	path: string,
	errors: string[],
): ValueOf<N> | undefined {
	switch (node.kind) {
		case "bool": {
			const b = parseBool(yamlValue);
			if (yamlValue !== undefined && b === undefined)
				errors.push(`Invalid boolean at ${path}`);
			return b as ValueOf<N>;
		}
		case "num": {
			const n = parseNum(yamlValue);
			if (yamlValue !== undefined && n === undefined)
				errors.push(`Invalid number at ${path}`);
			return n as ValueOf<N>;
		}
		case "str": {
			const s = parseStr(yamlValue);
			if (yamlValue !== undefined && s === undefined)
				errors.push(`Invalid string at ${path}`);
			return s as ValueOf<N>;
		}
		case "enm": {
			const s = parseStr(yamlValue);
			if (
				yamlValue !== undefined &&
				(s === undefined ||
					!node.values.includes(s as (typeof node.values)[number]))
			) {
				errors.push(
					`Invalid value at ${path}, expected one of: ${node.values.join(", ")}`,
				);
			}
			return s as ValueOf<N>;
		}
		case "arr": {
			const a = parseStrArray(yamlValue, node.sep);
			if (yamlValue !== undefined && a === undefined)
				errors.push(`Invalid array at ${path}`);
			return a as ValueOf<N>;
		}
		case "anyobj": {
			if (yamlValue === undefined) return undefined;
			if (!isPlainObject(yamlValue)) {
				errors.push(`Expected object at ${path}`);
				return undefined;
			}
			return yamlValue as ValueOf<N>;
		}
		case "obj": {
			if (yamlValue === undefined) return undefined;
			if (!isPlainObject(yamlValue)) {
				errors.push(`Expected object at ${path}`);
				return undefined;
			}
			// unknown key detection
			for (const k of Object.keys(yamlValue)) {
				if (!(k in node.shape)) errors.push(`Unknown option: ${path}${k}`);
			}
			const out: Record<string, unknown> = {};
			for (const [k, child] of Object.entries(node.shape)) {
				const childPath = path ? `${path}${k}.` : `${k}.`;
				const v = fromYaml(
					child,
					(yamlValue as Record<string, unknown>)[k],
					childPath,
					errors,
				);
				if (v !== undefined) out[k] = v;
			}
			return out as ValueOf<N>;
		}
	}
}

function fromEnv<N extends SchemaNode>(
	node: N,
	env: Env,
): ValueOf<N> | undefined {
	switch (node.kind) {
		case "bool": {
			if (!node.env) return undefined;
			const value = parseBool(env[node.env]);
			return value === undefined ? undefined : (value as ValueOf<N>);
		}
		case "num": {
			if (!node.env) return undefined;
			const value = parseNum(env[node.env]);
			return value === undefined ? undefined : (value as ValueOf<N>);
		}
		case "str": {
			if (!node.env) return undefined;
			const parsed = parseStr(env[node.env]);
			if (typeof parsed === "string" && parsed.trim().length > 0) {
				return parsed as ValueOf<N>;
			}
			return undefined;
		}
		case "enm": {
			if (!node.env) return undefined;
			const s = parseStr(env[node.env]);
			if (s && node.values.includes(s as (typeof node.values)[number])) {
				return s as ValueOf<N>;
			}
			return undefined;
		}
		case "arr": {
			if (!node.env) return undefined;
			const parsed = parseStrArray(env[node.env], node.sep);
			return parsed === undefined ? undefined : (parsed as ValueOf<N>);
		}
		case "anyobj":
			return undefined;
		case "obj": {
			const out: Record<string, unknown> = {};
			for (const [k, child] of Object.entries(node.shape)) {
				const v = fromEnv(child, env);
				if (v !== undefined) out[k] = v;
			}
			return Object.keys(out).length ? (out as ValueOf<N>) : undefined;
		}
	}
}

function withDefaults<N extends SchemaNode>(node: N): ValueOf<N> {
	switch (node.kind) {
		case "bool":
			return node.default as ValueOf<N>;
		case "num":
			return node.default as ValueOf<N>;
		case "str":
			return (node.default ?? undefined) as ValueOf<N>;
		case "enm":
			return node.default as ValueOf<N>;
		case "arr":
			return node.default as ValueOf<N>;
		case "anyobj":
			return node.default as ValueOf<N>;
		case "obj": {
			const out: Record<string, unknown> = {};
			for (const [k, child] of Object.entries(node.shape)) {
				out[k] = withDefaults(child);
			}
			return out as ValueOf<N>;
		}
	}
}

function applyConstraints<N extends SchemaNode>(
	node: N,
	value: ValueOf<N>,
	path: string,
	errors: string[],
): void {
	if (node.kind === "num" && node.gt !== undefined) {
		if (
			typeof (value as unknown) === "number" &&
			(value as number) <= node.gt
		) {
			errors.push(`Number at ${path} must be > ${node.gt}`);
		}
	}
	if (node.kind === "obj") {
		const record = value as Record<string, unknown>;
		for (const [k, child] of Object.entries(node.shape)) {
			const childValue = record[k] as ValueOf<typeof child>;
			applyConstraints(child, childValue, `${path}${k}.`, errors);
		}
	}
}

function merge<N extends SchemaNode>(
	node: N,
	yaml: ValueOf<N> | undefined,
	envV: ValueOf<N> | undefined,
): ValueOf<N> {
	switch (node.kind) {
		case "obj": {
			const yamlRecord = (yaml as Record<string, unknown>) ?? {};
			const envRecord = (envV as Record<string, unknown>) ?? {};
			const out: Record<string, unknown> = {};
			for (const [k, child] of Object.entries(node.shape)) {
				const childYaml = yamlRecord[k] as ValueOf<typeof child> | undefined;
				const childEnv = envRecord[k] as ValueOf<typeof child> | undefined;
				out[k] = merge(child, childYaml, childEnv);
			}
			return out as ValueOf<N>;
		}
		default:
			return (envV ?? yaml ?? withDefaults(node)) as ValueOf<N>;
	}
}

// Public: schema for Config
export const configSchema = obj({
	headless: bool({ env: "PUPPETEER_HEADLESS", default: true }),
	timeout: num({ env: "PUPPETEER_TIMEOUT", default: 30000, gt: 0 }),
	fileCapture: obj({
		enabled: bool({ env: "FILE_CAPTURE_ENABLED", default: false }),
		types: arr("str", {
			env: "FILE_CAPTURE_TYPES",
			sep: ",",
			default: ["script", "document", "stylesheet"],
		}),
		contentTypeMatchers: arr("str", {
			env: "FILE_CAPTURE_CT_MATCHERS",
			sep: ",",
			default: [
				"application/javascript",
				"application/x-javascript",
				"text/javascript",
				"application/ecmascript",
				"text/ecmascript",
				"application/json",
				"application/xml",
				"+xml",
			],
		}),

		maxFileSize: num({
			env: "FILE_CAPTURE_MAX_SIZE",
			default: 1024 * 1024,
			gt: 0,
		}),
		storage: enm(["memory", "redis", "cloud"] as const, {
			env: "FILE_CAPTURE_STORAGE",
			default: "memory",
		}),
		storageConfig: anyobj({ default: {} }), // validated conditionally downstream
	}),
	shipping: obj({
		endpoint: str({ env: "SHIPPING_ENDPOINT" }),
		batchSize: num({ env: "SHIPPING_BATCH_SIZE", default: 100, gt: 0 }),
		maxBatchAge: num({ env: "SHIPPING_MAX_BATCH_AGE", default: 5000, gt: 0 }),
	}),
	clientMonitoring: obj({
		enabled: bool({ env: "CLIENT_MONITORING_ENABLED", default: true }),
		events: arr("str", {
			env: "CLIENT_MONITORING_EVENTS",
			sep: ",",
			default: ["storage", "dynamicCode"],
		}),
	}),
	worker: obj({
		apiBaseUrl: str({ env: "MERRYMAKER_API_BASE" }),
		jobType: enm(["browser", "rules"] as const, {
			env: "WORKER_JOB_TYPE",
			default: "browser",
		}),
		leaseSeconds: num({ env: "WORKER_LEASE_SECONDS", default: 30, gt: 0 }),
		waitSeconds: num({ env: "WORKER_WAIT_SECONDS", default: 25, gt: 0 }),
		heartbeatSeconds: num({
			env: "WORKER_HEARTBEAT_SECONDS",
			default: 10,
			gt: 0,
		}),
	}),
	// Schemaless Puppeteer launch options (YAML only; not bound to env)
	launch: anyobj({ default: {} }),
});

export function loadFromSources<N extends SchemaNode>(
	node: N,
	opts: { yaml?: unknown; env: Env },
): ValueOf<N> {
	const errors: string[] = [];
	const y = fromYaml(node, opts.yaml, "", errors);
	if (errors.length)
		throw new Error(`Invalid YAML config: ${errors.join("; ")}`);
	const e = fromEnv(node, opts.env);
	const merged = merge(node, y, e);
	const constraints: string[] = [];
	applyConstraints(node, merged, "", constraints);
	if (constraints.length)
		throw new Error(`Invalid configuration: ${constraints.join("; ")}`);
	return merged;
}

export type LoadedConfig = Required<Config>;
