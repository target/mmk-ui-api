/**
 * Screenshot Modal Helper
 *
 * Specialized modal for displaying screenshots in full size.
 * Uses the Modal component for consistent behavior and accessibility.
 *
 * @example
 * ```js
 * import { showScreenshotModal } from './components/screenshot-modal.js';
 *
 * showScreenshotModal('data:image/png;base64,...', 'Screenshot caption');
 * ```
 */

import { h } from "../core/dom.js";
import { Modal } from "./modal.js";

/**
 * Show a screenshot in a modal dialog
 * @param {string} src - Image source URL or data URI
 * @param {string} caption - Optional image caption
 * @returns {Modal} The modal instance
 */
export function showScreenshotModal(src, caption = "") {
	// Use title for proper ARIA labeling (creates h3 with id="screenshot-modal-title")
	return Modal.show({
		id: "screenshot-modal",
		title: caption || "Screenshot",
		content: h("img", {
			src,
			alt: caption || "Screenshot",
			style: {
				maxWidth: "100%",
				maxHeight: "70vh",
				display: "block",
				margin: "0 auto",
			},
			onError: (e) => {
				const errorMsg = h("p", {
					text: "Failed to load image",
					style: {
						color: "#dc3545",
						textAlign: "center",
						padding: "20px",
					},
				});
				e.target.replaceWith(errorMsg);
			},
		}),
		buttons: [
			{
				text: "Close",
				className: "btn btn-secondary",
				onClick: (modal) => modal.hide(),
			},
		],
	});
}

export default showScreenshotModal;
