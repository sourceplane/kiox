const sidebars = {
  docsSidebar: [
    'intro',
    {
      type: 'category',
      label: 'Concepts',
      items: [
        'concepts/workspace',
        'concepts/providers',
        'concepts/runtime-shell',
        'concepts/caching',
        'concepts/execution-model',
      ],
    },
    {
      type: 'category',
      label: 'Getting Started',
      items: ['getting-started/installation', 'getting-started/quick-start'],
    },
    {
      type: 'category',
      label: 'CLI',
      items: [
        'cli/kiox',
        'cli/kiox-install',
        'cli/kiox-run',
        'cli/kiox-workspace',
        'cli/kiox-provider',
      ],
    },
    {
      type: 'category',
      label: 'Providers',
      items: [
        'providers/writing-providers',
        'providers/provider-packaging',
        'providers/provider-examples',
      ],
    },
    {
      type: 'category',
      label: 'Examples',
      items: [
        'examples/run-node',
        'examples/run-kubectl',
        'examples/ci-environment',
        'examples/multi-provider-workspace',
      ],
    },
    {
      type: 'category',
      label: 'Architecture',
      items: [
        'architecture/internals',
        'architecture/workspace-runtime',
        'architecture/provider-execution',
      ],
    },
    {
      type: 'category',
      label: 'Reference',
      items: ['reference/configuration', 'reference/environment-variables'],
    },
    {
      type: 'category',
      label: 'Contributing',
      items: ['contributing/contributing', 'contributing/writing-providers'],
    },
  ],
};

export default sidebars;
