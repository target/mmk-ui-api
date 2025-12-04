import { createLogger, type LoggerOptions } from "./lib/logger.js";

const isProd = process.env.NODE_ENV === "production";
const level = process.env.LOG_LEVEL || (isProd ? "info" : "debug");
const prettyEnv = process.env.LOG_PRETTY;
const prettyFlag = prettyEnv?.toLowerCase();
const disablePretty = new Set(["false", "0", "no", "off", "disabled"]);
const prettyEnabled =
	!isProd && (prettyFlag == null || !disablePretty.has(prettyFlag));

const redactPaths = [
	// generic secrets
	"authorization",
	"password",
	"token",
	"apiKey",
	"clientSecret",
	"secret",
	// common locations
	"headers.authorization",
	"headers.cookie",
	"cookies",
	// PII-ish
	"ssn",
	"creditCard.number",
	"cvv",
];

const baseOptions: LoggerOptions = {
	level: level as LoggerOptions["level"],
	redact: redactPaths,
	splitErrorStream: true,
};

const logger = (() => {
	if (prettyEnabled) {
		baseOptions.pretty = true;
	}
	return createLogger(baseOptions);
})();

const pageLogger = (() => {
	const opts: LoggerOptions = {
		...baseOptions,
		name: "browser",
		// ensure browser console logs do not mix with app errors
		splitErrorStream: false,
		stream: process.stdout,
	};
	return createLogger(opts);
})();

export { logger, pageLogger };
