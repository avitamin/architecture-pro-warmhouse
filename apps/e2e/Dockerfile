FROM node:24.15.0-alpine3.22
WORKDIR /runner
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts
COPY *.mjs ./
CMD ["npm", "run", "test:legacy"]
