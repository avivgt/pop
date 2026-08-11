FROM quay.io/operator-framework/opm:v1.23.0
ENTRYPOINT ["/bin/opm", "serve", "/configs"]
ADD catalog /configs
LABEL operators.operatorframework.io.index.configs.v1=/configs
