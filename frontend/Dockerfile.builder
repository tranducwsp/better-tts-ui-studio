FROM node:20-alpine

WORKDIR /app

ARG VITE_UI_MODE=beauty
ENV VITE_UI_MODE=$VITE_UI_MODE

COPY package*.json ./
RUN npm install

COPY . .

EXPOSE 3001

CMD ["npm", "run", "builder"]
