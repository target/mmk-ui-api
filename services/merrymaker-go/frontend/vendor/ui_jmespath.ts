// JMESPath vendor bundle
// Exports jmespath for browser use (only needed on alert sink forms)

// Import and re-export jmespath
import { search } from "@jmespath-community/jmespath";

// Attach to window for global access (matching CDN behavior)
declare global {
	interface Window {
		jmespath: {
			search: typeof search;
		};
	}
}

// Make jmespath available globally (matches unpkg behavior)
window.jmespath = {
	search,
};
