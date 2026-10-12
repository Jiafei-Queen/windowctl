// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLlmsTxt from 'starlight-llms-txt';

// GitHub Pages project site: https://muthuishere.github.io/windowctl/.
// Hand-written links in content carry the /windowctl/ prefix.
export default defineConfig({
	site: 'https://muthuishere.github.io',
	base: '/windowctl',
	integrations: [
		starlight({
			title: 'windowctl',
			description:
				'Move, resize, focus and tile windows across every monitor — one static binary on macOS, Windows and Linux. Plus screenshots, mouse & keyboard, on-screen arrows (draw), find-by-text, and browser remote control.',
			tableOfContents: false,
			plugins: [
				starlightLlmsTxt({
					projectName: 'windowctl',
					description:
						'A cross-platform CLI and Go library for window and monitor management, plus desktop automation: screenshots, input, on-screen arrows, find-by-text and browser remote control.',
					details:
						'windowctl lists and moves windows across monitors by zone, split, or coordinates; reports which monitor is active and which window is focused; applies whole layouts in one batch call; plus point-normalized screenshots, mouse/keyboard injection with a focus guard, launch/wait, a bigarrow-style draw overlay, find-by-text via accessibility tree then OCR, and a browser screen-share remote. One static binary per platform, distributed via npm.',
				}),
			],
			customCss: [
				'@fontsource-variable/inter',
				'@fontsource/jetbrains-mono/400.css',
				'@fontsource/jetbrains-mono/600.css',
				'./src/styles/windowctl.css',
			],
			social: [
				{ icon: 'github', label: 'GitHub', href: 'https://github.com/muthuishere/windowctl' },
			],
			sidebar: [
				{
					label: 'Start here',
					items: [
						{ label: 'Quickstart', slug: 'quickstart' },
						{ label: 'Zones & splits', slug: 'zones' },
						{ label: 'Monitors', slug: 'monitors' },
					],
				},
				{
					label: 'Window control',
					items: [
						{ label: 'Listing windows', slug: 'windows' },
						{ label: 'Move, resize, focus', slug: 'move' },
						{ label: 'Layouts in one call', slug: 'batch' },
					],
				},
				{
					label: 'Desktop automation',
					items: [
						{ label: 'Overview', slug: 'devices' },
						{ label: 'Screenshots', slug: 'capture' },
						{ label: 'Keyboard & mouse', slug: 'input' },
						{ label: 'Draw: point at things', slug: 'draw' },
						{ label: 'Find on screen', slug: 'find' },
						{ label: 'Remote control', slug: 'remote' },
					],
				},
				{
					label: 'Reference',
					items: [
						{ label: 'Agent skill', slug: 'agent-skill' },
						{ label: 'Go library', slug: 'library' },
						{ label: 'Permissions', slug: 'permissions' },
						{ label: 'How it is built', slug: 'internals' },
					],
				},
			],
		}),
	],
});
