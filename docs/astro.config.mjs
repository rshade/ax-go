// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import sitemap from '@astrojs/sitemap';
import starlightLinksValidator from 'starlight-links-validator';

// https://astro.build/config
export default defineConfig({
	site: 'https://rshade.github.io',
	base: '/ax-go',
	integrations: [
		starlight({
			title: 'ax-go',
			logo: { src: './src/assets/ax-go-mark.svg' },
			customCss: ['./src/styles/theme-bridge.css'],
			social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/rshade/ax-go' }],
			plugins: [starlightLinksValidator()],
			sidebar: [
				{ label: 'Tutorials', items: [{ autogenerate: { directory: 'tutorials' } }] },
				{ label: 'How-to Guides', items: [{ autogenerate: { directory: 'guides' } }] },
				{ label: 'Reference', items: [{ label: 'Sources', slug: 'sources' }] },
				{ label: 'Explanation', items: [{ autogenerate: { directory: 'explanation' } }] },
			],
		}),
		sitemap({ filter: (page) => !/\/adr\//.test(page) }),
	],
});
