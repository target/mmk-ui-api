import type { StorageEventPayload } from "./types.js";

type ClientEventData = Record<string, unknown>;

export interface ClientEvent {
	type: string;
	timestamp: number;
	// Deliberately permissive; client script may add fields per event type
	// Narrower types can be added later without breaking callers
	data: ClientEventData;
}

export const CLIENT_EVENT_METHOD_MAP = {
	"Storage.localStorageWrite": "Storage.localStorageWrite",
	"Storage.localStorageRead": "Storage.localStorageRead",
	"Storage.sessionStorageWrite": "Storage.sessionStorageWrite",
	"Storage.sessionStorageRead": "Storage.sessionStorageRead",
	"Runtime.dynamicCodeEval": "Runtime.dynamicCodeEval",
	"Runtime.dynamicCodeFunction": "Runtime.dynamicCodeFunction",
	"Security.apiTampering": "Security.apiTampering",
	"Security.monitoringInitialized": "Security.monitoringInitialized",
} as const;

export type ClientEventType = keyof typeof CLIENT_EVENT_METHOD_MAP;

export function mapClientEventToMethod(eventType: string): string {
	return (
		CLIENT_EVENT_METHOD_MAP[eventType as ClientEventType] ??
		`Client.${eventType}`
	);
}

export type ClientEventPayload = StorageEventPayload | ClientEventData;

export function mapClientEventPayload(
	clientEvent: ClientEvent,
): ClientEventPayload {
	if (clientEvent.type.startsWith("Storage.")) {
		const data = clientEvent.data as Record<string, unknown>;
		const key = resolveString(data.key);
		const value = resolveString(data.value);
		const rawValueSize = data.valueSize;
		const valueSize =
			typeof rawValueSize === "number" && Number.isFinite(rawValueSize)
				? rawValueSize
				: undefined;
		const payload: StorageEventPayload = {
			operation: getStorageOperation(clientEvent.type),
			storageType: getStorageType(clientEvent.type),
			key,
			value,
			valueSize,
		};
		return payload;
	}
	return clientEvent.data;
}

function resolveString(value: unknown): string | undefined {
	if (typeof value === "string") return value;
	if (typeof value === "number" || typeof value === "boolean") {
		return String(value);
	}
	return undefined;
}

export function getStorageOperation(
	eventType: string,
): "read" | "write" | "delete" | "clear" {
	if (eventType.includes("Write")) return "write";
	if (eventType.includes("Read")) return "read";
	if (eventType.includes("Delete")) return "delete";
	if (eventType.includes("Clear")) return "clear";
	return "write";
}

export function getStorageType(
	eventType: string,
): "localStorage" | "sessionStorage" {
	return eventType.includes("localStorage") ? "localStorage" : "sessionStorage";
}

export function getClientEventCategory(
	eventType: string,
): "network" | "security" | "runtime" | "page" | "dom" | "storage" {
	if (eventType.startsWith("Storage.")) return "storage";
	if (eventType.startsWith("Security.")) return "security";
	if (eventType.startsWith("Runtime.")) return "runtime";
	return "runtime";
}
