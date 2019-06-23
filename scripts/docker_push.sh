#!/bin/bash
docker login -u "$DOCKER_USERNAME" -p "$DOCKER_PASSWORD";
docker tag centeredge/shawarma:${TRAVIS_BUILD_NUMBER} centeredge/shawarma:${TRAVIS_TAG};
docker tag centeredge/shawarma:${TRAVIS_BUILD_NUMBER} centeredge/shawarma:latest;
docker push centeredge/shawarma:${TRAVIS_TAG};
docker push centeredge/shawarma:latest;
