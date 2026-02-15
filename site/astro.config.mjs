import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  integrations: [
    starlight({
      title: '🍄 Kinoko',
      customCss: ['./src/styles/custom.css'],
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
