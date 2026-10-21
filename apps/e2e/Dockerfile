FROM node:24.15.0-alpine3.22
WORKDIR /runner
COPY apps/e2e/package.json apps/e2e/package-lock.json ./
RUN npm ci --ignore-scripts
COPY apps/e2e/*.mjs ./
COPY docs/api/schemas.json ./
CMD ["npm", "run", "test:legacy"]
