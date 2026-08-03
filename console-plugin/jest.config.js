module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'jsdom',
  roots: ['<rootDir>/src'],
  setupFilesAfterEnv: ['<rootDir>/jest.setup.ts'],
  moduleNameMapper: {
    // Icons are mocked to avoid pulling the large icon barrel into tests.
    '@patternfly/react-icons': '<rootDir>/src/__mocks__/patternfly-icons.ts',
    // Force the CJS build of react-core; jest's resolver otherwise picks the
    // ESM entry, which is not transformed and yields undefined named exports.
    '^@patternfly/react-core$':
      '<rootDir>/node_modules/@patternfly/react-core/dist/js/index.js',
    '\\.(css|scss|less)$': '<rootDir>/src/__mocks__/style.ts',
  },
};
