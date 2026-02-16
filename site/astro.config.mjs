import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  output: 'static',
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
        github: 'https://github.com/kinshitai/kinoko',
      },
      sidebar: [
        {
          label: 'Getting Started',
          items: [
            { label: 'What is Kinoko?', slug: 'what-is-kinoko' },
            { label: 'Quickstart', slug: 'quickstart' },
            { label: 'Installation', slug: 'installation' },
          ],
        },
        {
          label: 'Concepts',
          items: [
            { label: 'Overview', slug: 'concepts/overview' },
            { label: 'Architecture', slug: 'concepts/architecture' },
            { label: 'Security', slug: 'concepts/security' },
            { label: 'Extraction', slug: 'concepts/extraction' },
            { label: 'Quality', slug: 'concepts/quality' },
            { label: 'Injection', slug: 'concepts/injection' },
            { label: 'Decay', slug: 'concepts/decay' },
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
          label: 'Project',
          items: [
            { label: 'Manifesto', slug: 'manifesto' },
          ],
        },
      ],
    }),
  ],
});
