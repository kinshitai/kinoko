import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://kinoko.tech',
  integrations: [
    starlight({
      title: '🍄 Kinoko',
      description: 'Knowledge-sharing infrastructure for AI agents. Every problem solved once is solved for everyone.',
      customCss: ['./src/styles/custom.css'],
      favicon: '/favicon.svg',
      head: [
        {
          tag: 'meta',
          attrs: { property: 'og:title', content: 'Kinoko — Knowledge Infrastructure for AI Agents' },
        },
        {
          tag: 'meta',
          attrs: { property: 'og:description', content: 'Every problem solved once is solved for everyone. Knowledge-sharing infrastructure for AI agents.' },
        },
        {
          tag: 'meta',
          attrs: { property: 'og:type', content: 'website' },
        },
        {
          tag: 'meta',
          attrs: { property: 'og:url', content: 'https://kinoko.tech' },
        },
      ],
      social: {
        github: 'https://github.com/kinoko-dev/kinoko',
      },
      sidebar: [
        {
          label: 'Getting Started',
          items: [
            { label: 'Quickstart', slug: 'quickstart' },
            { label: 'Installation', slug: 'installation' },
          ],
        },
        {
          label: 'Concepts',
          items: [
            { label: 'Overview', slug: 'concepts/overview' },
            { label: 'Extraction (Gold Panning)', slug: 'concepts/extraction' },
            { label: 'Quality (Wine Tasting)', slug: 'concepts/quality' },
            { label: 'Injection (Reference Librarian)', slug: 'concepts/injection' },
            { label: 'Decay (Forest Fires)', slug: 'concepts/decay' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { label: 'CLI', slug: 'reference/cli' },
            { label: 'Configuration', slug: 'reference/config' },
            { label: 'Skill Format', slug: 'reference/skill-format' },
            { label: 'Glossary', slug: 'reference/glossary' },
          ],
        },
        {
          label: 'Operations',
          items: [
            { label: 'Troubleshooting', slug: 'operations/troubleshooting' },
          ],
        },
        {
          label: 'Manifesto',
          slug: 'manifesto',
        },
      ],
    }),
  ],
});
