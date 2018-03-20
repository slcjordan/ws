const path = require('path');

module.exports = {
    entry: './js/ws.js',
    output: {
        filename: 'ws-bundle.js',
        path: path.resolve(__dirname, 'dist')
    },
    resolve: {
        modules: [path.resolve(__dirname, 'vendor', 'node_modules')]
    }
};
