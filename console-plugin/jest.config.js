module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  roots: ['<rootDir>/src'],
  moduleNameMapper: {
    '@patternfly/react-icons': '<rootDir>/src/__mocks__/patternfly-icons.ts',
  },
};
