# Stage 1: Build production static bundle
FROM node:20-alpine AS builder

WORKDIR /app

ARG VITE_UI_MODE=beauty
ENV VITE_UI_MODE=$VITE_UI_MODE

COPY package*.json ./
RUN npm install

COPY . .
RUN npm run build

# Stage 2: Serve static bundle via high-performance Nginx
FROM nginx:alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 5173

CMD ["nginx", "-g", "daemon off;"]
